package ticket

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

func (p *Postgres) Revise(ctx context.Context, in ReviseInput) (Ticket, bool, error) {
	if err := validateRevise(in); err != nil {
		return Ticket{}, false, err
	}
	tx, ticket, err := p.lockedTicket(ctx, in.Ref.Key)
	if err != nil {
		return Ticket{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if ticket.RequestedBy != in.By {
		return Ticket{}, false, ErrNotRequester
	}
	if replayed, err := replayedEvent(ctx, tx, in.EventID, in.Ref.Key); err != nil || replayed {
		return ticket, false, err
	}
	if err := requireMutable(ticket, in.Ref); err != nil {
		return Ticket{}, false, err
	}
	change := revisionChange{
		eventID: in.EventID, draft: in.Draft,
		recipients: ticket.Current.Recipients, at: in.At,
	}
	return replaceRevision(ctx, tx, ticket, change)
}

func (p *Postgres) Address(ctx context.Context, in AddressInput) (Ticket, bool, error) {
	recipients, err := validateAddress(in)
	if err != nil {
		return Ticket{}, false, err
	}
	tx, ticket, err := p.lockedTicket(ctx, in.Ref.Key)
	if err != nil {
		return Ticket{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if ticket.AddressedBy == "" || ticket.AddressedBy != in.By {
		return Ticket{}, false, ErrNotAddressSource
	}
	if replayed, err := replayedEvent(ctx, tx, in.EventID, in.Ref.Key); err != nil || replayed {
		return ticket, false, err
	}
	if err := requireMutable(ticket, in.Ref); err != nil {
		return Ticket{}, false, err
	}
	if slices.Equal(ticket.Current.Recipients, recipients) {
		return recordAddressingEvent(ctx, tx, ticket, in)
	}
	change := revisionChange{
		eventID: in.EventID, draft: ticket.Current.Draft,
		recipients: recipients, at: in.At,
	}
	return replaceRevision(ctx, tx, ticket, change)
}

func replayedEvent(
	ctx context.Context, tx pgx.Tx, eventID string, key domain.TicketKey,
) (bool, error) {
	ref, exists, err := eventRef(ctx, tx, eventID)
	if err != nil || !exists {
		return false, err
	}
	if ref.Key != key {
		return false, ErrEventTaken
	}
	return true, nil
}

func recordAddressingEvent(
	ctx context.Context, tx pgx.Tx, ticket Ticket, in AddressInput,
) (Ticket, bool, error) {
	if err := insertEvent(ctx, tx, in.EventID, in.Ref, in.At); err != nil {
		return Ticket{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Ticket{}, false, fmt.Errorf("ticket: commit addressing event: %w", err)
	}
	return ticket, false, nil
}
