package ticket

import (
	"context"
)

/*
CloseTicket ends a request.

Idempotent on purpose: two people reacting at the same instant, or the same
reaction delivered twice, is the ordinary case and neither is a conflict. The
first close is the one recorded, because it is the one that happened.
*/
func (m *Memory) CloseTicket(ctx context.Context, in CloseTicketInput) (Ticket, bool, error) {
	if err := validateCloseTicket(in); err != nil {
		return Ticket{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return Ticket{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	held, exists := m.tickets[in.Key]
	if !exists {
		return Ticket{}, false, ErrNotFound
	}
	if held.Closed != nil {
		return cloneTicket(held), false, nil
	}
	at := in.At.UTC()
	held.Closed, held.ClosedBy, held.UpdatedAt = &at, in.By, at
	m.tickets[in.Key] = held
	return cloneTicket(held), true, nil
}
