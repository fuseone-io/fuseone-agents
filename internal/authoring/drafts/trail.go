package drafts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
	"github.com/jackc/pgx/v5"
)

type trailRow struct {
	draftID string
	agent   domain.AgentID
	action  string
	// body is what the change said; nil for a discard, which says nothing.
	body []byte
	// kept is a body already in the content store, an accepted proposal's:
	// the trail names it rather than storing a copy, so erasing the thread
	// the proposal came from reaches the trail's body too.
	kept     *keptBody
	origin   draft.Origin
	revision domain.Revision
	by       domain.UserID
}

func originName(o draft.Origin) string {
	if o == draft.FromProposal {
		return "proposal"
	}
	return "control"
}

func scopeOf(d draft.Draft) domain.Scope {
	return domain.Scope{Company: d.Spec.Company, Area: d.Spec.Area}
}

// creationBody is what opening a draft said: the identity of a new agent's
// birth, or the version a published agent's draft opened from.
func creationBody(in insertion) []byte {
	body, _ := json.Marshal(struct {
		Kind    string `json:"kind"`
		Name    string `json:"name,omitempty"`
		Company string `json:"company"`
		Area    string `json:"area"`
		From    string `json:"from,omitempty"`
	}{"create", in.d.Spec.Name, string(in.d.Spec.Company), string(in.d.Spec.Area), string(in.base)})
	return body
}

/*
appendTrail writes the next row of a draft's trail, and the change's body.

The body goes to run_content under owner kind authoring, in this
transaction and exactly: its size is judged here, and the store is unbounded,
so it is never truncated. The installation's retention and erasure reach it
there; the immutable row keeps only its reference and digest. The draft's row
lock serialises writers of one draft, so the next seq is read safely.
*/
func (s *Store) appendTrail(ctx context.Context, tx pgx.Tx, t trailRow) error {
	var seq int64
	if err := tx.QueryRow(ctx, `
		select coalesce(max(seq), 0) + 1 from agent_draft_trail where draft_id = $1::uuid`,
		t.draftID).Scan(&seq); err != nil {
		return fmt.Errorf("drafts: trail of %s: %w", t.agent, err)
	}
	var digest, ref string
	switch {
	case t.kept != nil:
		digest, ref = t.kept.digest, t.kept.ref
	case t.body != nil:
		if len(t.body) > draft.MaxDefinitionBytes {
			return fmt.Errorf("%w: a change of %d bytes", draft.ErrTooLarge, len(t.body))
		}
		sum := sha256.Sum256(t.body)
		digest = hex.EncodeToString(sum[:])
		stored, err := s.content.RestoreForTx(ctx, tx, "authoring", t.draftID, seq, t.body)
		if err != nil {
			return fmt.Errorf("drafts: keep the body of %s: %w", t.agent, err)
		}
		ref = stored
	}
	if _, err := tx.Exec(ctx, `
		insert into agent_draft_trail (draft_id, seq, agent_id, action, digest, body_ref, origin, revision, by_user, at)
		values ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, now())`,
		t.draftID, seq, string(t.agent), t.action, digest, ref, originName(t.origin),
		string(t.revision), string(t.by)); err != nil {
		return fmt.Errorf("drafts: trail of %s: %w", t.agent, err)
	}
	return nil
}
