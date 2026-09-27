package channel

import (
	"context"
	"fmt"
	"slices"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ticket"
)

/*
Corrections written where the answer is reviewed.

A ticket with a review room is worked there before anything reaches the person
who asked: the draft is posted for decision, somebody who can decide replies
with what is wrong, and that reply is a revision like any other — the pending
approval is superseded and the agent writes the answer again.

Who may correct is the scope's approvers, all of them, and the check is made
here rather than in the store for the reason recipients are: the store knows
what a ticket is, not who holds a role today. Everybody else may talk in the
room without starting anything, which is what makes it usable as a room.
*/
func (h *TicketHandler) correct(
	ctx context.Context, arrival Claimed, held ticket.Ticket,
) (TicketResult, error) {
	if !arrival.Source.Person() {
		// The room's own cards arrive here too. A bot correcting the draft it
		// just wrote is a loop with a budget.
		return handled("ticket_source_ignored"), nil
	}
	who, linked, err := h.bindings.PrincipalFor(ctx, arrival.Channel, arrival.Source.User)
	if err != nil {
		return TicketResult{}, fmt.Errorf("channel: resolve ticket reviewer: %w", err)
	}
	if !linked {
		return handled("ticket_reviewer_ignored"), nil
	}
	allowed, err := h.deciders.DecidersIn(ctx, held.Scope)
	if err != nil {
		return TicketResult{}, fmt.Errorf("channel: resolve ticket deciders: %w", err)
	}
	if !slices.Contains(allowed, who) {
		return handled("ticket_reviewer_ignored"), nil
	}
	return h.revise(ctx, arrival, held, who)
}

/*
ReviewPlace answers where a ticket's own messages belong.

The room it is worked in, and the thread inside it once there is one. Before
that thread exists — a mark refused for want of a requester, say — the room
itself, because the one place the answer must not appear is the thread the
person who asked is reading. Without a room configured at all there is nothing
to redirect, and the arrival's own thread is where it was always said.
*/
func (h *TicketHandler) ReviewPlace(
	ctx context.Context, intent *TicketIntent,
) (conversation, thread string, ok bool) {
	if h == nil || h.store == nil || intent == nil {
		return "", "", false
	}
	if held, err := h.store.Current(ctx, intent.Key); err == nil && held.Review.Open() {
		return held.Review.Conversation, held.Review.Root, true
	}
	if intent.ReviewIn != "" {
		return intent.ReviewIn, "", true
	}
	return "", "", false
}

/*
keep folds what the person who asked added into their own request.

They may still say things — a detail they forgot, an answer to a question — and
none of it starts work: what regenerates an answer is a correction in the room,
where somebody is deciding. Their words are still the request, so they are kept
rather than read and dropped.

Nothing is folded in while a decision is pending. A revision written then
supersedes the card somebody is reading, and nothing would regenerate to
replace it: the room would be left holding a button that decides a request that
has moved. Such a message is recorded and goes no further.
*/
func (h *TicketHandler) keep(
	ctx context.Context, arrival Claimed, held ticket.Ticket, who domain.UserID,
) (TicketResult, error) {
	if held.Current.Phase != ticket.PhaseCollecting {
		return handled("ticket_context_not_applied"), nil
	}
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
	if _, _, err := h.store.Revise(ctx, ticket.ReviseInput{
		Ref: held.Current.Ref, EventID: arrival.EventID,
		By: who, Draft: draft, At: h.now().UTC(),
	}); err != nil {
		return TicketResult{}, err
	}
	return handled("ticket_context_kept"), nil
}
