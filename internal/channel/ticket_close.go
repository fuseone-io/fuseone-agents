package channel

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/fuseone/agents/internal/ticket"
)

/*
Ending a request with a reaction.

A team says a chamado is done by putting an emoji on it, and this is the whole
of that: the emoji has to be one the room configured, the message has to be the
request itself, and the person has to be somebody who may decide there. Anything
else is somebody reacting to a message, which is what people do all day.

Only adding counts. A reaction taken away is not an instruction — reopening
work by removing an emoji is a path nobody tests and everybody triggers by
accident — and it never reaches here.
*/
func (r *TicketRoutes) closing(
	ctx context.Context, candidate TicketCandidate,
) (TicketIntent, bool, error) {
	if r.tickets == nil || candidate.Source.User == "" {
		return TicketIntent{}, false, nil
	}
	held, err := r.tickets.AtOrigin(ctx, ticket.Origin{
		Connection: candidate.Connection, Conversation: candidate.Conversation,
		Root: candidate.Message,
	})
	if errors.Is(err, ticket.ErrNotFound) {
		return TicketIntent{}, false, nil
	}
	if err != nil {
		return TicketIntent{}, false, fmt.Errorf("channel: find ticket to close: %w", err)
	}
	admitted, ok, err := r.admission(ctx, candidate)
	if err != nil || !ok {
		return TicketIntent{}, false, err
	}
	if !admitted.compiled.rule.ClosesTicket(candidate.Reaction) {
		return r.ignored(candidate, notClosingEmoji)
	}
	return TicketIntent{Key: held.Key, Close: true}, true, nil
}

/*
close ends the request, if the person reacting may.

Approver in the ticket's scope and nobody else: a reaction is one click from
anybody in the room, and ending somebody's request is a decision. Whoever may
not is ignored in silence, because a channel where a joking emoji answers back
is a channel people stop using.
*/
func (h *TicketHandler) close(
	ctx context.Context, arrival Claimed, held ticket.Ticket,
) (TicketResult, error) {
	who, linked, err := h.bindings.PrincipalFor(ctx, arrival.Channel, arrival.Source.User)
	if err != nil {
		return TicketResult{}, fmt.Errorf("channel: resolve who closed the ticket: %w", err)
	}
	if !linked {
		return handled("ticket_closer_ignored"), nil
	}
	allowed, err := h.deciders.DecidersIn(ctx, held.Scope)
	if err != nil {
		return TicketResult{}, fmt.Errorf("channel: resolve ticket deciders: %w", err)
	}
	if !slices.Contains(allowed, who) {
		return handled("ticket_closer_ignored"), nil
	}
	closed, changed, err := h.store.CloseTicket(ctx, ticket.CloseTicketInput{
		Key: held.Key, By: who, At: h.now().UTC(),
	})
	if err != nil {
		return TicketResult{}, err
	}
	if !changed {
		return handled("ticket_already_closed"), nil
	}
	return h.dropPendingAnswer(ctx, closed)
}

/*
dropPendingAnswer takes the button away from a decision that no longer decides.

An answer waiting for approval when the request ends must not stay decidable:
approving it later would publish into a thread somebody has already finished
with. The card is settled as failed and the room is told, because a card that
simply stops working is a card somebody keeps pressing.
*/
func (h *TicketHandler) dropPendingAnswer(
	ctx context.Context, held ticket.Ticket,
) (TicketResult, error) {
	if held.Current.Phase != ticket.PhaseAwaitingApproval || held.Current.Approval == nil {
		return handled("ticket_closed"), nil
	}
	if err := h.closeSupersededApproval(ctx, *held.Current.Approval); err != nil {
		return TicketResult{}, err
	}
	if _, _, err := h.store.Close(ctx, ticket.CloseInput{
		Ref: held.Current.Ref, Phase: ticket.PhaseCancelled,
		Result: held.Current.Draft, At: h.now().UTC(),
	}); err != nil {
		return TicketResult{}, err
	}
	return TicketResult{Refusal: Refusal{
		Why:    "This ticket was closed. The answer that was waiting for approval was not published.",
		Reason: "ticket_closed_with_answer_pending",
	}}, nil
}
