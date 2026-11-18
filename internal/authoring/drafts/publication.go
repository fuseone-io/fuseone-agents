package drafts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
A draft published from the workbench (NT-018, contract 4).

The publication is the ordinary one, with its fences and its claim; what the
draft adds is judged in the same transaction, once the version is current and
before the commit: the caller still reaches the draft, the draft is the one
they read, and its base becomes the version it was published as. Taken after
the event graph's lock and the agent's row, so the order every writer takes
stays one: graph, agent, draft.
*/

// Publishing is a draft about to be published.
type Publishing struct {
	Agent domain.AgentID
	// Expect is the draft's ETag the caller read and is publishing.
	Expect domain.Revision
	Caller Caller
}

// WithinPublication is the draft's condition on a publication, to run in its
// transaction (spec.Publication.Within): a draft that moved since it was read
// answers a stale precondition, and the publication is undone.
func (s *Store) WithinPublication(p Publishing) func(context.Context, pgx.Tx, domain.VersionID) error {
	return func(ctx context.Context, tx pgx.Tx, version domain.VersionID) error {
		r, err := s.lockReachable(ctx, tx, p.Agent, p.Caller)
		if err != nil {
			return err
		}
		if r.revision != p.Expect {
			return &domain.StaleError{Current: r.revision}
		}
		if _, err := tx.Exec(ctx, `
			update agent_drafts set base_version = $2, updated_by = $3, updated_at = now() where draft_id = $1::uuid`,
			r.draftID, string(version), string(p.Caller.By)); err != nil {
			return fmt.Errorf("drafts: base the draft on %s: %w", version, err)
		}
		return nil
	}
}
