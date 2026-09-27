package channel

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/ticket"
)

// errReviewPending says the ticket has a room and the room has no thread yet.
// Not a failure: the sweep that opens it runs beside this one, and the card
// waits rather than going where the room exists to keep it from.
var errReviewPending = errors.New("channel: the ticket's review room has no thread yet")

func ticketApproval(report Report) bool {
	return report.Ticket.Valid() && report.Event == EventParked &&
		report.AwaitingDecision && report.AtSeq > 0
}

// ticketReport is anything a run of a ticket has to say. All of it belongs
// where the ticket is worked: the room if it has one, and its own thread
// otherwise. Never at the top of the channel the person who asked is reading,
// which is what "a ticket conversation" means.
func ticketReport(report Report) bool { return report.Ticket.Valid() }

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
	if !ticketReport(report) {
		return places, nil
	}
	route, err := f.ticketRoute(ctx, report)
	if err != nil {
		return nil, err
	}
	room, err := ticketRoom(route)
	if err != nil {
		return nil, err
	}
	// One place, and it is the ticket's. Another conversation covering the
	// scope would repeat where nobody is working the ticket, and the support
	// thread would carry work the person who asked was never meant to read:
	// only an approved answer reaches them, and it is published by the
	// outcome, not by an announcement.
	return []Conversation{{
		Channel: room.Connection, ID: room.Conversation,
		Label: room.Conversation, Thread: room.Root, Wants: []Event{report.Event},
	}}, nil
}

/*
ticketRoom answers where this ticket's decision is taken.

The support thread, unless the ticket was opened with a room of its own — and
then only that room. A card in both places would put a half-written answer in
front of the person who asked for it, which is the one thing the room exists to
prevent.
*/
func ticketRoom(route ticket.ApprovalRoute) (ticket.Origin, error) {
	if route.Review.Conversation == "" {
		return route.Origin, nil
	}
	if !route.Review.Open() {
		return ticket.Origin{}, errReviewPending
	}
	return ticket.Origin{
		Connection:   route.Origin.Connection,
		Conversation: route.Review.Conversation, Root: route.Review.Root,
	}, nil
}

func (f *fanout) isTicketRoom(report Report, place Conversation) bool {
	if !ticketApproval(report) {
		return false
	}
	held, ok := f.byTicket[report.Ticket]
	if !ok || held.err != nil {
		return false
	}
	room, err := ticketRoom(held.value)
	return err == nil && place.Channel == room.Connection &&
		place.ID == room.Conversation && place.Thread == room.Root
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
