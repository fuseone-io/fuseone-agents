package ticket

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

func (p *Postgres) lockedTicket(
	ctx context.Context, key domain.TicketKey,
) (pgx.Tx, Ticket, error) {
	tx, err := p.beginLocked(ctx, key)
	if err != nil {
		return nil, Ticket{}, err
	}
	ticket, err := readTicket(ctx, tx, key, true)
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, Ticket{}, err
	}
	return tx, ticket, nil
}

func replaceRevision(
	ctx context.Context, tx pgx.Tx, ticket Ticket, change revisionChange,
) (Ticket, bool, error) {
	if ticket.Current.Phase != PhaseExecuting && ticket.Current.Phase != PhaseNeedsAttention {
		if err := cancelRevision(ctx, tx, ticket.Current.Ref, change.at); err != nil {
			return Ticket{}, false, err
		}
	}
	ref := domain.TicketRef{Key: ticket.Key, Revision: ticket.Current.Ref.Revision + 1}
	if err := insertRevision(ctx, tx, Revision{
		Ref: ref, Phase: PhaseCollecting, Draft: change.draft,
		Recipients: change.recipients,
		CreatedAt:  change.at.UTC(), UpdatedAt: change.at.UTC(),
	}); err != nil {
		return Ticket{}, false, err
	}
	if err := insertEvent(ctx, tx, change.eventID, ref, change.at); err != nil {
		return Ticket{}, false, err
	}
	if err := advanceTicket(ctx, tx, ref, change.at); err != nil {
		return Ticket{}, false, err
	}
	return commitTicket(ctx, tx, ticket.Key, true)
}

func touchTicket(ctx context.Context, tx pgx.Tx, key domain.TicketKey, at time.Time) error {
	_, err := tx.Exec(ctx, `update governed_tickets set updated_at = $2 where ticket_key = $1`,
		string(key), at.UTC())
	if err != nil {
		return fmt.Errorf("ticket: touch: %w", err)
	}
	return nil
}

func commitTicket(
	ctx context.Context, tx pgx.Tx, key domain.TicketKey, changed bool,
) (Ticket, bool, error) {
	ticket, err := readTicket(ctx, tx, key, false)
	if err != nil {
		return Ticket{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Ticket{}, false, fmt.Errorf("ticket: commit: %w", err)
	}
	return ticket, changed, nil
}

func cancelRevision(ctx context.Context, tx pgx.Tx, ref domain.TicketRef, at time.Time) error {
	_, err := tx.Exec(ctx, `
		update governed_ticket_revisions set phase = $3, updated_at = $4
		where ticket_key = $1 and revision = $2`,
		string(ref.Key), ref.Revision, string(PhaseCancelled), at.UTC())
	if err != nil {
		return fmt.Errorf("ticket: cancel old revision: %w", err)
	}
	return nil
}

func advanceTicket(ctx context.Context, tx pgx.Tx, ref domain.TicketRef, at time.Time) error {
	_, err := tx.Exec(ctx, `
		update governed_tickets set current_revision = $2, updated_at = $3
		where ticket_key = $1`, string(ref.Key), ref.Revision, at.UTC())
	if err != nil {
		return fmt.Errorf("ticket: advance revision: %w", err)
	}
	return nil
}
