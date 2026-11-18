package drafts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
)

// Discard removes a draft, against the tag the caller read. Its trail stays.
func (s *Store) Discard(ctx context.Context, agent domain.AgentID, expect domain.IfMatch, c Caller) error {
	if !expect.Present() {
		return ErrPreconditionRequired
	}
	return s.within(ctx, func(tx pgx.Tx) error {
		r, err := s.lockReachable(ctx, tx, agent, c)
		if err != nil {
			return err
		}
		if !expect.Allows(true, r.revision) {
			return &domain.StaleError{Current: r.revision}
		}
		if _, err := tx.Exec(ctx, `delete from agent_drafts where agent_id = $1`, string(agent)); err != nil {
			return fmt.Errorf("drafts: discard %s: %w", agent, err)
		}
		_, err = s.appendTrail(ctx, tx, trailRow{
			draftID: r.draftID, agent: agent, action: "discard",
			origin: draft.FromControl, revision: r.revision, by: c.By,
		})
		return err
	})
}
