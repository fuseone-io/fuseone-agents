package ticket

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// CloseTicket ends a request. Idempotent: the first close is the one recorded,
// and a second says so without failing.
func (p *Postgres) CloseTicket(ctx context.Context, in CloseTicketInput) (Ticket, bool, error) {
	if err := validateCloseTicket(in); err != nil {
		return Ticket{}, false, err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return Ticket{}, false, fmt.Errorf("ticket: begin close: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	held, err := readTicket(ctx, tx, in.Key, true)
	if err != nil {
		return Ticket{}, false, err
	}
	if held.Closed != nil {
		return held, false, tx.Commit(ctx)
	}
	if _, err := tx.Exec(ctx, `
		update governed_tickets
		set closed_at = $2, closed_by = $3, updated_at = $2
		where ticket_key = $1 and closed_at is null`,
		string(in.Key), in.At.UTC(), string(in.By)); err != nil {
		return Ticket{}, false, fmt.Errorf("ticket: close: %w", err)
	}
	closed, err := readTicket(ctx, tx, in.Key, false)
	if err != nil {
		return Ticket{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		if errors.Is(err, pgx.ErrTxClosed) {
			return closed, true, nil
		}
		return Ticket{}, false, fmt.Errorf("ticket: commit close: %w", err)
	}
	return closed, true, nil
}
