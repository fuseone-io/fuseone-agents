package channel

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/ticket"
)

func ticketApproval(report Report) bool {
	return report.Ticket.Valid() && report.Event == EventParked &&
		report.AwaitingDecision && report.AtSeq > 0
}

func (f *fanout) ticketRoute(
	ctx context.Context, report Report,
) (ticket.ApprovalRoute, error) {
	if before, asked := f.byTicket[report.Ticket]; asked {
		return before.value, before.err
	}
	if f.tickets == nil {
		err := errors.New("channel: ticket approval routes are not configured")
		f.byTicket[report.Ticket] = answered[ticket.ApprovalRoute]{err: err}
		return ticket.ApprovalRoute{}, err
	}
	route, err := f.tickets.ApprovalRoute(ctx, report.Ticket)
	if err == nil && !route.Origin.Valid() {
		err = errors.New("channel: ticket approval route is invalid")
	}
	f.byTicket[report.Ticket] = answered[ticket.ApprovalRoute]{value: route, err: err}
	return route, err
}

func (f *fanout) ticketPlaces(
	ctx context.Context, report Report, places []Conversation,
) ([]Conversation, error) {
	if !ticketApproval(report) {
		return places, nil
	}
	route, err := f.ticketRoute(ctx, report)
	if err != nil {
		return nil, err
	}
	found := false
	out := append([]Conversation(nil), places...)
	for i := range out {
		if out[i].Channel != route.Origin.Connection ||
			out[i].ID != route.Origin.Conversation {
			continue
		}
		found = true
		out[i].Thread = route.Origin.Root
		out[i].Agent = ""
		out[i].Wants = []Event{EventParked}
		out[i].DirectApprovals = false
	}
	if !found {
		out = append(out, Conversation{
			Channel: route.Origin.Connection, ID: route.Origin.Conversation,
			Label: route.Origin.Conversation, Thread: route.Origin.Root,
			Wants: []Event{EventParked},
		})
	}
	return out, nil
}

func (f *fanout) isTicketRoom(report Report, place Conversation) bool {
	if !ticketApproval(report) {
		return false
	}
	held, ok := f.byTicket[report.Ticket]
	return ok && held.err == nil &&
		place.Channel == held.value.Origin.Connection &&
		place.ID == held.value.Origin.Conversation &&
		place.Thread == held.value.Origin.Root
}

// directForTicket tells only the people selected by the ticket's configured
// addressing source. The list narrows current authority; it never grants it.
func (r *Reporter) directForTicket(
	ctx context.Context, pass *fanout, report Report,
) (sent int, refused refusal) {
	route, err := pass.ticketRoute(ctx, report)
	if err != nil {
		return 0, refusal{blocking: []error{err},
			recorded: r.failuresFor(report, Conversation{}, err)}
	}
	if len(route.Recipients) == 0 {
		return 0, refusal{}
	}
	place := Conversation{Channel: route.Origin.Connection, DirectApprovals: true}
	who, err := pass.decidersAmong(ctx, report.Scope, route.Recipients)
	if err != nil {
		return 0, refusal{blocking: []error{err}, recorded: r.failuresFor(report, place, err)}
	}
	if len(who) == 0 {
		return 0, r.refuse(report, place, NewError(CodeNamedNobodyWhoDecides,
			"channel: the people named by this ticket cannot decide in the run's scope"))
	}

	to, capped, err := pass.reachable(ctx, place, who)
	switch {
	case err != nil:
		return 0, refusal{blocking: []error{err}, recorded: r.failuresFor(report, place, err)}
	case capped:
		return 0, r.refuse(report, place, NewError(CodeTooManyRecipients,
			"channel: more ticket recipients can be reached than one approval should reach"))
	case len(to) == 0:
		return 0, r.refuse(report, place, NewError(CodeNobodyReachable,
			"channel: nobody named by this ticket has linked an account on its connection"))
	}

	for _, person := range to {
		posted, postErr := r.post(ctx, report, person)
		if postErr != nil {
			refused.recorded = append(refused.recorded, r.failuresFor(report, person, postErr)...)
			if !degrades(postErr) {
				refused.blocking = append(refused.blocking,
					fmt.Errorf("channel: tell ticket recipient %s privately: %w", person.ID, postErr))
			}
			continue
		}
		if posted {
			sent++
		}
	}
	return sent, refused
}
