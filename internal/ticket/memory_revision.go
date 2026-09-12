package ticket

import (
	"context"
	"slices"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

type revisionChange struct {
	eventID    string
	draft      ContentRef
	recipients []domain.UserID
	at         time.Time
}

func (m *Memory) Revise(ctx context.Context, in ReviseInput) (Ticket, bool, error) {
	if err := validateRevise(in); err != nil {
		return Ticket{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ticket, err := m.ticketFor(ctx, in.Ref.Key)
	if err != nil {
		return Ticket{}, false, err
	}
	if ticket.RequestedBy != in.By {
		return Ticket{}, false, ErrNotRequester
	}
	if replayed, err := m.replayed(in.EventID, in.Ref.Key); err != nil || replayed {
		return cloneTicket(ticket), false, err
	}
	if err := requireMutable(ticket, in.Ref); err != nil {
		return Ticket{}, false, err
	}
	change := revisionChange{
		eventID: in.EventID, draft: in.Draft,
		recipients: ticket.Current.Recipients, at: in.At,
	}
	return m.replace(ticket, change)
}

func (m *Memory) Address(ctx context.Context, in AddressInput) (Ticket, bool, error) {
	recipients, err := validateAddress(in)
	if err != nil {
		return Ticket{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ticket, err := m.ticketFor(ctx, in.Ref.Key)
	if err != nil {
		return Ticket{}, false, err
	}
	if ticket.AddressedBy == "" || ticket.AddressedBy != in.By {
		return Ticket{}, false, ErrNotAddressSource
	}
	if replayed, err := m.replayed(in.EventID, in.Ref.Key); err != nil || replayed {
		return cloneTicket(ticket), false, err
	}
	if err := requireMutable(ticket, in.Ref); err != nil {
		return Ticket{}, false, err
	}
	if slices.Equal(ticket.Current.Recipients, recipients) {
		m.events[in.EventID] = in.Ref
		return cloneTicket(ticket), false, nil
	}
	change := revisionChange{
		eventID: in.EventID, draft: ticket.Current.Draft,
		recipients: recipients, at: in.At,
	}
	return m.replace(ticket, change)
}

func (m *Memory) replayed(eventID string, key domain.TicketKey) (bool, error) {
	ref, exists := m.events[eventID]
	if !exists {
		return false, nil
	}
	if ref.Key != key {
		return false, ErrEventTaken
	}
	return true, nil
}

func (m *Memory) replace(ticket Ticket, change revisionChange) (Ticket, bool, error) {
	if ticket.Current.Phase != PhaseExecuting {
		previous := ticket.Current
		previous.Phase, previous.UpdatedAt = PhaseCancelled, change.at.UTC()
		m.revisions[ticket.Key][previous.Ref.Revision] = previous
	}
	ref := domain.TicketRef{Key: ticket.Key, Revision: ticket.Current.Ref.Revision + 1}
	revision := Revision{
		Ref: ref, Phase: PhaseCollecting, Draft: change.draft,
		Recipients: append([]domain.UserID(nil), change.recipients...),
		CreatedAt:  change.at.UTC(), UpdatedAt: change.at.UTC(),
	}
	ticket.Current, ticket.UpdatedAt = revision, change.at.UTC()
	m.tickets[ticket.Key] = ticket
	m.revisions[ticket.Key][ref.Revision] = revision
	m.events[change.eventID] = ref
	return cloneTicket(ticket), true, nil
}
