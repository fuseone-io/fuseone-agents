package ticket

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

func (p *Postgres) ClaimReviews(
	ctx context.Context, owner string, now time.Time, lease time.Duration, limit int,
) ([]ReviewNotice, error) {
	if err := validateReviewClaim(owner, now, lease, limit); err != nil {
		return nil, err
	}
	rows, err := p.pool.Query(ctx, `
		with due as (
			select ticket_key
			from governed_tickets
			where review_conversation <> '' and review_root = ''
			  and (review_claim_until is null or review_claim_until <= $2)
			order by updated_at, ticket_key
			for update skip locked
			limit $4
		)
		update governed_tickets t
		set review_claimed_by = $1, review_claim_until = $3
		from due
		where t.ticket_key = due.ticket_key
		returning t.ticket_key, t.origin_connection, t.origin_conversation, t.origin_root,
		          t.review_conversation, t.company_id, t.area_id, t.requested_by`,
		owner, now.UTC(), now.Add(lease).UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("ticket: claim review rooms: %w", err)
	}
	defer rows.Close()
	var out []ReviewNotice
	for rows.Next() {
		var notice ReviewNotice
		var key, company, area, requester string
		if err := rows.Scan(&key, &notice.Origin.Connection, &notice.Origin.Conversation,
			&notice.Origin.Root, &notice.Conversation, &company, &area, &requester); err != nil {
			return nil, fmt.Errorf("ticket: read review room: %w", err)
		}
		notice.Key = domain.TicketKey(key)
		notice.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
		notice.RequestedBy = domain.UserID(requester)
		if !notice.Origin.Valid() || !notice.Scope.Valid() || notice.Conversation == "" {
			return nil, errors.New("ticket: stored review room is invalid")
		}
		out = append(out, notice)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ticket: iterate review rooms: %w", err)
	}
	return out, nil
}

func (p *Postgres) MarkReviewOpened(
	ctx context.Context, key domain.TicketKey, root, owner string, at time.Time,
) error {
	if err := validateReviewMark(key, root, owner, at); err != nil {
		return err
	}
	tag, err := p.pool.Exec(ctx, `
		update governed_tickets
		set review_root = case when review_root = '' then $2 else review_root end,
		    review_claimed_by = '', review_claim_until = null, updated_at = $4
		where ticket_key = $1 and review_conversation <> ''
		  and (review_claimed_by = $3 or review_root <> '')`,
		string(key), root, owner, at.UTC())
	if err != nil {
		return fmt.Errorf("ticket: mark review thread opened: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMoved
	}
	return nil
}

var _ ReviewRooms = (*Postgres)(nil)
