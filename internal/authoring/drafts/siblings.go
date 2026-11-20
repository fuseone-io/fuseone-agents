package drafts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
stillTheDraftRead is contract 3 for a proposal: the draft is the one, at the ETag, the
model read, or one only its own turn's acceptances moved since (#412). Every proposal of
a turn names the ETag the turn read, so the first acceptance would otherwise leave every
sibling stale. Anything else between — a change by hand, another turn's proposal, a write
from a replica that kept no provenance — is still a 412.
*/
func stillTheDraftRead(ctx context.Context, tx pgx.Tx, p stored, r row) error {
	if r.draftID != p.Draft {
		return &domain.StaleError{Current: r.revision}
	}
	if r.revision == p.Read {
		return nil
	}
	if p.Turn == "" {
		return &domain.StaleError{Current: r.revision}
	}
	siblings, err := movedOnlyByTurn(ctx, tx, r.draftID, p.Read, p.Turn)
	if err != nil {
		return err
	}
	if !siblings {
		return &domain.StaleError{Current: r.revision}
	}
	return nil
}

// movedOnlyByTurn is whether the draft was last at read and every change after it is the
// acceptance of a proposal of turn. The draft's ETag names its content, so read is the
// last time the draft held what the model read.
func movedOnlyByTurn(ctx context.Context, tx pgx.Tx, draftID string, read domain.Revision, turn string) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, `
		with anchor as (
			select max(seq) as seq from agent_draft_trail where draft_id = $1::uuid and revision = $2
		)
		select (select seq from anchor) is not null and not exists (
			select 1 from agent_draft_trail t
			left join agent_draft_trail_provenance v on v.draft_id = t.draft_id and v.trail_seq = t.seq
			left join agent_draft_proposals p on p.proposal_id = v.proposal_id
			where t.draft_id = $1::uuid and t.seq > (select seq from anchor)
			  and (t.origin <> 'proposal' or p.turn_id is distinct from $3::uuid)
		)`, draftID, string(read), turn).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("drafts: read what moved the draft since the turn: %w", err)
	}
	return ok, nil
}

// waitsOnThisDraft is whether p is valid, undecided and still on the draft r holds.
func waitsOnThisDraft(ctx context.Context, tx pgx.Tx, p stored, r row) (bool, error) {
	if !p.Valid || p.Decision != "" {
		return false, nil
	}
	err := stillTheDraftRead(ctx, tx, p, r)
	var stale *domain.StaleError
	if errors.As(err, &stale) {
		return false, nil
	}
	return err == nil, err
}
