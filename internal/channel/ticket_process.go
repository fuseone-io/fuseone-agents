package channel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ticket"
)

type TicketContent interface {
	PutFor(context.Context, string, string, int64, []byte) (string, error)
	Get(context.Context, string) ([]byte, error)
}

type TicketBindings interface {
	PrincipalFor(context.Context, string, string) (domain.UserID, bool, error)
}

type TicketRuns interface {
	Read(context.Context, domain.RunID, int64) ([]domain.Step, error)
	AppendIfHead(context.Context, domain.StepRef, domain.Step) (domain.Step, error)
}

type TicketResult struct {
	RunID         domain.RunID
	Refusal       Refusal
	HandledReason string
}

// TicketHandler turns one persisted routing decision into a ticket transition
// and, when that transition names a request revision, one idempotent run.
type TicketHandler struct {
	store     ticket.Store
	content   TicketContent
	opener    Opens
	bindings  TicketBindings
	addresses TicketAddresses
	deciders  TicketDeciders
	runs      TicketRuns
	now       func() time.Time
}

// WithTickets wires the governed-ticket path. It is optional for consumers
// that can never receive a persisted TicketIntent; such an arrival fails
// closed instead of falling through to the ordinary ask path.
func (c *Consumer) WithTickets(tickets *TicketHandler) *Consumer {
	c.tickets = tickets
	return c
}

func (c *Consumer) handleTicket(ctx context.Context, claimed Claimed) (bool, error) {
	if c.tickets == nil {
		return false, fmt.Errorf("%w: a ticket handler", ErrNotWired)
	}
	result, err := c.tickets.Handle(ctx, claimed)
	if err != nil {
		return false, err
	}
	switch {
	case result.RunID != "":
		if err := c.inbox.OpenedTicket(ctx, claimed, string(result.RunID), c.clock()); err != nil {
			return false, err
		}
		return true, nil
	case result.Refusal.Why != "":
		return false, c.decline(ctx, claimed, result.Refusal)
	case result.HandledReason != "":
		return false, c.inbox.Handled(ctx, claimed, result.HandledReason, c.clock())
	default:
		return false, errors.New("channel: ticket handler returned no disposition")
	}
}

func NewTicketHandler(
	store ticket.Store, content TicketContent, opener Opens,
	bindings TicketBindings, addresses TicketAddresses,
	deciders TicketDeciders, runs TicketRuns, now func() time.Time,
) *TicketHandler {
	if now == nil {
		now = time.Now
	}
	return &TicketHandler{
		store: store, content: content, opener: opener,
		bindings: bindings, addresses: addresses, deciders: deciders, runs: runs, now: now,
	}
}

func (h *TicketHandler) Handle(ctx context.Context, arrival Claimed) (TicketResult, error) {
	if h == nil || h.store == nil || h.content == nil || h.opener == nil ||
		h.bindings == nil || h.addresses == nil || h.deciders == nil || h.runs == nil {
		return TicketResult{}, fmt.Errorf("%w: governed ticket dependencies", ErrNotWired)
	}
	if arrival.Ticket == nil || !arrival.Ticket.Valid() {
		return TicketResult{}, errors.New("channel: invalid persisted ticket intent")
	}
	if held, revision, err := h.store.EventRevision(ctx, arrival.EventID); err == nil {
		return h.openRevision(ctx, held, revision)
	} else if !errors.Is(err, ticket.ErrNotFound) {
		return TicketResult{}, err
	}
	if arrival.Ticket.Root {
		return h.openRoot(ctx, arrival)
	}
	return h.handleReply(ctx, arrival)
}

func (h *TicketHandler) openRoot(ctx context.Context, arrival Claimed) (TicketResult, error) {
	who, linked, err := h.bindings.PrincipalFor(ctx, arrival.Channel, arrival.Source.User)
	if err != nil {
		return TicketResult{}, fmt.Errorf("channel: resolve ticket requester: %w", err)
	}
	if !linked {
		return TicketResult{Refusal: Refusal{
			Why:    "Link your Slack account to FuseOne before opening a governed ticket.",
			Reason: "ticket_unbound",
		}}, nil
	}
	raw, err := firstTicketDraft(arrival.Message, who, arrival.Text)
	if err != nil {
		return ticketContextRefusal(err), nil
	}
	draft, err := h.storeDraft(ctx, arrival.Ticket.Key, 1, raw)
	if err != nil {
		return TicketResult{}, err
	}
	held, _, err := h.store.Open(ctx, ticket.OpenInput{
		Key: arrival.Ticket.Key,
		Origin: ticket.Origin{Connection: arrival.Channel,
			Conversation: arrival.Conversation, Root: arrival.Thread},
		Scope: arrival.Ticket.Scope, Agent: arrival.Ticket.Agent, RunAs: arrival.Ticket.RunAs,
		RequestedBy: who, AddressedBy: arrival.Ticket.AddressedBy,
		EventID: arrival.EventID, Draft: draft, At: h.now().UTC(),
	})
	if errors.Is(err, ticket.ErrTooManyOpen) {
		return TicketResult{Refusal: Refusal{
			Why:    "This area already has too many open tickets. Finish one before opening another.",
			Reason: "ticket_scope_full",
		}}, nil
	}
	if errors.Is(err, ticket.ErrMoved) {
		return handled("ticket_root_already_open"), nil
	}
	if err != nil {
		return TicketResult{}, err
	}
	return h.openRevision(ctx, held, held.Current)
}

func (h *TicketHandler) handleReply(ctx context.Context, arrival Claimed) (TicketResult, error) {
	held, err := h.store.Current(ctx, arrival.Ticket.Key)
	if err != nil {
		return TicketResult{}, err
	}
	if held.Current.Phase == ticket.PhaseCompleted ||
		held.Current.Phase == ticket.PhaseRejected ||
		held.Current.Phase == ticket.PhaseCancelled {
		// Slack may deliver a later reply to a thread whose governed work is
		// already over. It is not a retryable store failure and it must not
		// reopen authority from an approval that has already been consumed.
		return handled("ticket_already_closed"), nil
	}
	if arrival.Source.MatchesKey(held.AddressedBy) {
		return h.address(ctx, arrival, held)
	}
	if arrival.Source.User == "" || arrival.Source.Bot != "" || arrival.Source.App != "" {
		return handled("ticket_source_ignored"), nil
	}
	who, linked, err := h.bindings.PrincipalFor(ctx, arrival.Channel, arrival.Source.User)
	if err != nil {
		return TicketResult{}, fmt.Errorf("channel: resolve ticket reply author: %w", err)
	}
	if !linked || who != held.RequestedBy {
		return handled("ticket_author_ignored"), nil
	}
	return h.revise(ctx, arrival, held, who)
}

func (h *TicketHandler) revise(
	ctx context.Context, arrival Claimed, held ticket.Ticket, who domain.UserID,
) (TicketResult, error) {
	for range 4 {
		previous, err := h.content.Get(ctx, held.Current.Draft.Ref)
		if err != nil {
			return TicketResult{}, fmt.Errorf("channel: read ticket context: %w", err)
		}
		raw, err := appendTicketDraft(previous, arrival.Message, who, arrival.Text)
		if err != nil {
			return ticketContextRefusal(err), nil
		}
		draft, err := h.storeDraft(ctx, held.Key, held.Current.Ref.Revision+1, raw)
		if err != nil {
			return TicketResult{}, err
		}
		updated, _, err := h.store.Revise(ctx, ticket.ReviseInput{
			Ref: held.Current.Ref, EventID: arrival.EventID,
			By: who, Draft: draft, At: h.now().UTC(),
		})
		if err == nil {
			return h.openRevision(ctx, updated, updated.Current)
		}
		if !errors.Is(err, ticket.ErrMoved) {
			return TicketResult{}, err
		}
		held, err = h.store.Current(ctx, held.Key)
		if err != nil {
			return TicketResult{}, err
		}
	}
	return TicketResult{}, ticket.ErrMoved
}

func handled(reason string) TicketResult { return TicketResult{HandledReason: reason} }
