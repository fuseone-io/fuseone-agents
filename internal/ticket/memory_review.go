package ticket

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

type reviewClaim struct {
	owner string
	until time.Time
}

func (m *Memory) ClaimReviews(
	ctx context.Context, owner string, now time.Time, lease time.Duration, limit int,
) ([]ReviewNotice, error) {
	if err := validateReviewClaim(owner, now, lease, limit); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	owed := make([]ReviewNotice, 0)
	for key, held := range m.tickets {
		if held.Review.Conversation == "" || held.Review.Root != "" {
			continue
		}
		if claim := m.reviewClaims[key]; claim.owner != "" && claim.until.After(now) {
			continue
		}
		owed = append(owed, ReviewNotice{
			Key: key, Origin: held.Origin, Conversation: held.Review.Conversation,
			Scope: held.Scope, RequestedBy: held.RequestedBy,
		})
	}
	slices.SortFunc(owed, func(a, b ReviewNotice) int {
		return cmp.Compare(string(a.Key), string(b.Key))
	})
	if len(owed) > limit {
		owed = owed[:limit]
	}
	for _, notice := range owed {
		m.reviewClaims[notice.Key] = reviewClaim{owner: owner, until: now.Add(lease)}
	}
	return owed, nil
}

func (m *Memory) MarkReviewOpened(
	ctx context.Context, key domain.TicketKey, root, owner string, at time.Time,
) error {
	if err := validateReviewMark(key, root, owner, at); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	held, exists := m.tickets[key]
	if !exists || held.Review.Conversation == "" {
		return ErrNotFound
	}
	// Already open is not a conflict: the thread exists, which is what the
	// claim was for. A different worker's claim is, for the same reason the
	// outcome notice refuses one.
	if held.Review.Root != "" {
		return nil
	}
	if claim := m.reviewClaims[key]; claim.owner != owner {
		return ErrMoved
	}
	held.Review.Root = strings.TrimSpace(root)
	held.UpdatedAt = at.UTC()
	m.tickets[key] = held
	delete(m.reviewClaims, key)
	return nil
}

var _ ReviewRooms = (*Memory)(nil)
