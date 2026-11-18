package drafts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

/*
sealReach is what an erasure of a draft's conversation must also reach once values are
sealed (NT-018 phase 2): every seal of the draft that copied a value whose source is one of
these threads or proposals, or whose source could not be proved, and the content of every
run derived from those seals or opened from those threads.

A seal is a snapshot, so it is judged by its own sources and not by the draft as it stands:
a value overwritten in the draft after it was sealed is still in the seal.
*/
type sealReach struct {
	draftID   string
	threads   []string
	proposals []string
	reason    string
}

// eraseSealsReachedTx erases the reached seals' bytes and the derived runs' content, in the
// caller's transaction, under the corpus fence it holds for the draft's agent.
func (s *Store) eraseSealsReachedTx(ctx context.Context, tx pgx.Tx, in sealReach) (int, error) {
	seals, err := sealsReached(ctx, tx, in)
	if err != nil {
		return 0, err
	}
	erased := 0
	if len(seals) > 0 {
		n, err := s.content.EraseOwnersTx(ctx, tx, "authoring", seals, in.reason)
		if err != nil {
			return 0, fmt.Errorf("drafts: erase sealed values: %w", err)
		}
		erased += n
	}
	runs, err := runsDerived(ctx, tx, seals, in.threads)
	if err != nil || len(runs) == 0 {
		return erased, err
	}
	n, err := s.content.EraseOwnersTx(ctx, tx, "run", runs, in.reason)
	if err != nil {
		return 0, fmt.Errorf("drafts: erase runs derived from erased values: %w", err)
	}
	return erased + n, nil
}

func sealsReached(ctx context.Context, tx pgx.Tx, in sealReach) ([]string, error) {
	rows, err := tx.Query(ctx, `
		select distinct s.seal_id::text
		from agent_seals s
		join agent_seal_sources source on source.seal_id = s.seal_id
		left join agent_draft_trail_provenance p
		  on p.draft_id = s.draft_id and p.trail_seq = source.trail_seq
		left join agent_draft_proposals proposal on proposal.proposal_id = p.proposal_id
		where s.draft_id = $1::uuid
		  and (p.trail_seq is null
		       or proposal.thread_id = any($2::uuid[])
		       or proposal.proposal_id = any($3::uuid[]))
		order by 1`, in.draftID, in.threads, in.proposals)
	if err != nil {
		return nil, fmt.Errorf("drafts: find seals reached by erasure: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func runsDerived(ctx context.Context, tx pgx.Tx, seals, threads []string) ([]string, error) {
	if len(seals) == 0 && len(threads) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		select run_id from runs
		where seal_id = any($1::uuid[]) or authoring_thread = any($2::uuid[])
		order by run_id`, seals, threads)
	if err != nil {
		return nil, fmt.Errorf("drafts: find runs derived from erased values: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}
