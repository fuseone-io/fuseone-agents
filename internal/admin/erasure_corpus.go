package admin

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

type corpusErasure struct {
	cases   int
	objects int
}

/*
eraseCorpusOf follows a named run into the corpus.

A correction keeps its own copy of the occurrence, filed under the agent so it
outlives the run, and a battery opens a new run holding the same input every
time it runs. Erasing only the run leaves all of those behind, with nothing to
tell the operator they exist.

Everything here is found by provenance, from what the run already recorded, so
no index by person comes into existence.

Two corrections made from identical input share one reference, because the
reference is the digest. Erasing it takes both: they hold the same bytes, and
keeping one of them would keep the bytes.
*/
func (e *Erasures) eraseCorpusOf(
	ctx context.Context, tx pgx.Tx, runs []domain.RunID,
) (corpusErasure, error) {
	cases, refs, err := corpusCasesFrom(ctx, tx, memoryRunIDs(runs))
	if err != nil {
		return corpusErasure{}, err
	}
	if len(cases) == 0 {
		return corpusErasure{}, nil
	}
	objects, err := e.content.EraseRefsTx(ctx, tx, refs, "subject")
	if err != nil {
		return corpusErasure{}, fmt.Errorf("admin: erase corpus copies: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`update regression_cases set note = '' where case_id = any($1::text[])`, cases); err != nil {
		return corpusErasure{}, fmt.Errorf("admin: clear corpus notes: %w", err)
	}
	replays, err := runsReplaying(ctx, tx, cases)
	if err != nil {
		return corpusErasure{}, err
	}
	replayed, err := e.eraseRuns(ctx, tx, replays)
	if err != nil {
		return corpusErasure{}, err
	}
	return corpusErasure{cases: len(cases), objects: objects + replayed}, nil
}

func corpusCasesFrom(
	ctx context.Context, tx pgx.Tx, runs []string,
) (cases, refs []string, err error) {
	rows, err := tx.Query(ctx, `
		select case_id, input_ref from regression_cases
		where from_run = any($1::text[])`, runs)
	if err != nil {
		return nil, nil, fmt.Errorf("admin: find corrections of erased runs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kase, ref string
		if err := rows.Scan(&kase, &ref); err != nil {
			return nil, nil, fmt.Errorf("admin: scan correction: %w", err)
		}
		cases, refs = append(cases, kase), append(refs, ref)
	}
	return cases, refs, rows.Err()
}

func runsReplaying(ctx context.Context, tx pgx.Tx, cases []string) ([]domain.RunID, error) {
	rows, err := tx.Query(ctx, `
		select r.run_id from runs r
		join run_steps s on s.run_id = r.run_id and s.seq = 1 and s.kind = 'run_started'
		where r.simulated and s.payload->>'case' = any($1::text[])`, cases)
	if err != nil {
		return nil, fmt.Errorf("admin: find replays of erased corrections: %w", err)
	}
	defer rows.Close()
	var out []domain.RunID
	for rows.Next() {
		var run string
		if err := rows.Scan(&run); err != nil {
			return nil, fmt.Errorf("admin: scan replay: %w", err)
		}
		out = append(out, domain.RunID(run))
	}
	return out, rows.Err()
}

// clearNotesOfErasedCases is retention's half: the copy aged out, and a note is
// free text about it. The case itself stays, as a visible gap in the corpus.
func (e *Erasures) clearNotesOfErasedCases(ctx context.Context, tx pgx.Tx) (int, error) {
	tag, err := tx.Exec(ctx, `
		update regression_cases c set note = ''
		from run_content rc
		where rc.ref = c.input_ref and rc.erased_at is not null and c.note <> ''`)
	if err != nil {
		return 0, fmt.Errorf("admin: clear notes of erased corrections: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
