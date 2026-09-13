package ticket

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

func (p *Postgres) ClaimOutcomes(
	ctx context.Context, owner string, now time.Time, lease time.Duration, limit int,
) ([]OutcomeNotice, error) {
	if err := validateOutcomeClaim(owner, now, lease, limit); err != nil {
		return nil, err
	}
	rows, err := p.pool.Query(ctx, `
		with due as (
			select r.ticket_key, r.revision
			from governed_ticket_revisions r
			join governed_tickets t on t.ticket_key = r.ticket_key
			where r.phase in ('completed', 'rejected', 'needs_attention') and r.outcome_ref <> ''
			  and r.outcome_announced_at is null
			  and (r.outcome_claim_until is null or r.outcome_claim_until <= $2)
			  and t.origin_connection <> '' and t.origin_conversation <> '' and t.origin_root <> ''
			order by r.updated_at, r.ticket_key, r.revision
			for update of r skip locked
			limit $4
		), claimed as (
			update governed_ticket_revisions r
			set outcome_claimed_by = $1, outcome_claim_until = $3
			from due
			where r.ticket_key = due.ticket_key and r.revision = due.revision
			returning r.ticket_key, r.revision, r.phase, r.outcome_ref, r.outcome_digest
		)
		select c.ticket_key, c.revision,
		       t.origin_connection, t.origin_conversation, t.origin_root,
		       c.phase, c.outcome_ref, c.outcome_digest
		from claimed c join governed_tickets t on t.ticket_key = c.ticket_key
		order by c.ticket_key, c.revision`, owner, now.UTC(), now.Add(lease).UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("ticket: claim outcome notifications: %w", err)
	}
	defer rows.Close()
	var out []OutcomeNotice
	for rows.Next() {
		var notice OutcomeNotice
		var key, phase string
		if err := rows.Scan(&key, &notice.Ref.Revision,
			&notice.Origin.Connection, &notice.Origin.Conversation, &notice.Origin.Root,
			&phase, &notice.Result.Ref, &notice.Result.Digest); err != nil {
			return nil, fmt.Errorf("ticket: read outcome notification: %w", err)
		}
		notice.Ref.Key, notice.Phase = domain.TicketKey(key), Phase(phase)
		if !notice.Ref.Valid() || !notice.Origin.Valid() || !notice.Result.Valid() ||
			(notice.Phase != PhaseCompleted && notice.Phase != PhaseRejected &&
				notice.Phase != PhaseNeedsAttention) {
			return nil, errors.New("ticket: stored outcome notification is invalid")
		}
		out = append(out, notice)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ticket: iterate outcome notifications: %w", err)
	}
	return out, nil
}

func (p *Postgres) MarkOutcomeAnnounced(
	ctx context.Context, ref domain.TicketRef, owner string, at time.Time,
) error {
	if err := validateOutcomeMark(ref, owner, at); err != nil {
		return err
	}
	tag, err := p.pool.Exec(ctx, `
		update governed_ticket_revisions
		set outcome_announced_at = coalesce(outcome_announced_at, $4),
		    outcome_claimed_by = '', outcome_claim_until = null
		where ticket_key = $1 and revision = $2
		  and (outcome_claimed_by = $3 or outcome_announced_at is not null)`,
		string(ref.Key), ref.Revision, owner, at.UTC())
	if err != nil {
		return fmt.Errorf("ticket: mark outcome announced: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMoved
	}
	return nil
}

var _ OutcomeNotices = (*Postgres)(nil)
