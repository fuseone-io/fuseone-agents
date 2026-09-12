package ticket

import (
	"context"
	"sync"

	"github.com/fuseone/agents/internal/domain"
)

type Memory struct {
	mu        sync.RWMutex
	tickets   map[domain.TicketKey]Ticket
	revisions map[domain.TicketKey]map[int64]Revision
	events    map[string]domain.TicketRef
}

func NewMemory() *Memory {
	return &Memory{
		tickets:   make(map[domain.TicketKey]Ticket),
		revisions: make(map[domain.TicketKey]map[int64]Revision),
		events:    make(map[string]domain.TicketRef),
	}
}

func (m *Memory) Open(ctx context.Context, in OpenInput) (Ticket, bool, error) {
	if err := ctx.Err(); err != nil {
		return Ticket{}, false, err
	}
	if err := validateOpen(in); err != nil {
		return Ticket{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if ref, exists := m.events[in.EventID]; exists {
		if ref.Key != in.Key {
			return Ticket{}, false, ErrEventTaken
		}
		return cloneTicket(m.tickets[in.Key]), false, nil
	}
	if current, exists := m.tickets[in.Key]; exists {
		return cloneTicket(current), false, ErrMoved
	}
	ref := domain.TicketRef{Key: in.Key, Revision: 1}
	revision := Revision{
		Ref: ref, Phase: PhaseCollecting, Draft: in.Draft,
		CreatedAt: in.At.UTC(), UpdatedAt: in.At.UTC(),
	}
	ticket := Ticket{
		Key: in.Key, Scope: in.Scope, RequestedBy: in.RequestedBy,
		AddressedBy: in.AddressedBy, Current: revision,
		CreatedAt: in.At.UTC(), UpdatedAt: in.At.UTC(),
	}
	m.tickets[in.Key] = ticket
	m.revisions[in.Key] = map[int64]Revision{1: revision}
	m.events[in.EventID] = ref
	return cloneTicket(ticket), true, nil
}

func (m *Memory) Current(ctx context.Context, key domain.TicketKey) (Ticket, error) {
	if err := ctx.Err(); err != nil {
		return Ticket{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	ticket, ok := m.tickets[key]
	if !ok {
		return Ticket{}, ErrNotFound
	}
	return cloneTicket(ticket), nil
}

func (m *Memory) Revision(ctx context.Context, ref domain.TicketRef) (Revision, error) {
	if err := ctx.Err(); err != nil {
		return Revision{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	revision, ok := m.revisions[ref.Key][ref.Revision]
	if !ok {
		return Revision{}, ErrNotFound
	}
	return cloneRevision(revision), nil
}
