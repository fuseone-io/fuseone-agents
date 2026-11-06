package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

type corpusErasure struct {
	cases   int
	objects int
}

// CorpusFence excludes anything copying these corpora, declared by the consumer.
//
// Recording a correction and opening a battery each read an occurrence and write it
// somewhere else. If the occurrence goes between the two, the second puts it back
// while this reports success, so everything here that makes one go runs inside this.
type CorpusFence interface {
	Exclusive(ctx context.Context, agents []domain.AgentID, inside func(context.Context) error) error
}

// agentsOf is whose corpora a set of runs reaches. Read before the fence and not in
// a transaction, because waiting at it must not hold what its work needs: a run's
// agent does not change, and a correction naming the run is for that same agent.
func (e *Erasures) agentsOf(ctx context.Context, runs []string) ([]domain.AgentID, error) {
	agents, err := scanStrings(ctx, e.pool, `
		select agent_id from runs where run_id = any($1::text[])
		union
		select agent_id from regression_cases where from_run = any($1::text[])`, runs)
	if err != nil {
		return nil, fmt.Errorf("admin: find the agents of erased runs: %w", err)
	}
	return toAgents(agents), nil
}

// expiredReplaysSQL is the rehearsals holding the bytes of a copy past the window. A
// battery is the platform duplicating those bytes, not anybody collecting them again,
// so a replay does not get a window of its own. A tombstoned copy still counts: the
// run that replayed it may be younger than whatever took it.
const expiredReplaysSQL = `
	select distinct held.run_id from run_content copied
	join run_content held on held.digest = copied.digest
	join runs r on r.run_id = held.run_id and r.agent_id = copied.run_id
	where copied.owner_kind = 'regression' and copied.created_at < $1
	  and held.owner_kind = 'run' and held.erased_at is null and r.simulated`

// eraseReplays takes the content of the runs a search found, in one statement, and
// says how many runs and objects that was.
//
// One statement because all of this runs holding the fence. Five years of nightly
// batteries is thousands of replays per case, and a command for each would keep
// corrections and batteries waiting on something proportional to history. Reading the
// ids out first would only move the same weight into memory.
func eraseReplays(
	ctx context.Context, tx pgx.Tx, search string, reason int, args ...any,
) (runs, objects int, err error) {
	statement := fmt.Sprintf(`
		with replays as (%s), erased as (
			update run_content c
			set bytes = null, erased_at = now(), erased_reason = $%d
			from replays r
			where c.owner_kind = 'run' and c.run_id = r.run_id and c.erased_at is null
			returning c.run_id
		)
		select count(distinct run_id), count(*) from erased`, search, reason)
	if err := tx.QueryRow(ctx, statement, args...).Scan(&runs, &objects); err != nil {
		return 0, 0, fmt.Errorf("admin: erase replays: %w", err)
	}
	return runs, objects, nil
}

// eraseExpiredReplays runs before the copies go, in the transaction that takes them.
func eraseExpiredReplays(ctx context.Context, tx pgx.Tx, before time.Time) (runs, objects int, err error) {
	return eraseReplays(ctx, tx, expiredReplaysSQL, 2, before.UTC(), "retention")
}

// agentsLosingCopies is whose corrections retention is about to reach.
func (e *Erasures) agentsLosingCopies(ctx context.Context, before time.Time) ([]domain.AgentID, error) {
	agents, err := scanStrings(ctx, e.pool, `
		select distinct run_id from run_content
		where owner_kind = 'regression' and created_at < $1 and erased_at is null`, before.UTC())
	if err != nil {
		return nil, fmt.Errorf("admin: find the corpora retention reaches: %w", err)
	}
	return toAgents(agents), nil
}

func toAgents(ids []string) []domain.AgentID {
	out := make([]domain.AgentID, 0, len(ids))
	for _, id := range ids {
		out = append(out, domain.AgentID(id))
	}
	return out
}

// Both start from a digest, on an index of what is still readable, so a request
// costs what it finds rather than what the installation has ever done.
const (
	copySearchSQL = `
		select ref from run_content
		where digest = any($1::text[]) and owner_kind = 'regression'
		  and run_id = any($2::text[]) and erased_at is null`
	replaySearchSQL = `
		select distinct rc.run_id from run_content rc
		join runs r on r.run_id = rc.run_id
		where rc.digest = any($1::text[]) and rc.owner_kind = 'run'
		  and rc.erased_at is null and r.simulated and r.agent_id = any($2::text[])`
)

/*
eraseCorpusOf follows a named run to everything that holds its input.

A correction keeps its own copy of the occurrence, filed under the agent so it
outlives the run, and a battery opens a new run holding the same input every
time it runs. Erasing only the run leaves all of those behind, with nothing to
tell the operator they exist.

It follows the digest rather than the case. A run recorded which input it was
opened with, and a tombstone keeps its digest, so the link survives the
correction being deleted and reaches every correction sharing that input. No
index by person comes into existence: this starts from runs somebody named.

Only inside the agents involved, and only into copies and rehearsals. A real run
holding the same bytes is a record of something that happened, and naming it is
the operator's act.
*/
func (e *Erasures) eraseCorpusOf(
	ctx context.Context, tx pgx.Tx, runs []string, agents []domain.AgentID,
) (corpusErasure, error) {
	digests, err := inputDigestsOf(ctx, tx, runs)
	if err != nil {
		return corpusErasure{}, err
	}
	owners := make([]string, 0, len(agents))
	for _, agent := range agents {
		owners = append(owners, string(agent))
	}
	refs, err := scanStrings(ctx, tx, copySearchSQL, digests, owners)
	if err != nil {
		return corpusErasure{}, fmt.Errorf("admin: find corpus copies: %w", err)
	}
	objects, err := e.content.EraseRefsTx(ctx, tx, refs, "subject")
	if err != nil {
		return corpusErasure{}, fmt.Errorf("admin: erase corpus copies: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		update regression_cases set note = ''
		where input_ref = any($1::text[]) or from_run = any($2::text[])`, refs, runs)
	if err != nil {
		return corpusErasure{}, fmt.Errorf("admin: clear corpus notes: %w", err)
	}
	_, replayed, err := eraseReplays(ctx, tx, replaySearchSQL, 3, digests, owners, "subject")
	if err != nil {
		return corpusErasure{}, err
	}
	return corpusErasure{cases: int(tag.RowsAffected()), objects: objects + replayed}, nil
}

// inputDigestsOf is the input each run recorded, and the copy any correction made
// from it. They differ when an input was too large to keep whole.
func inputDigestsOf(ctx context.Context, tx pgx.Tx, runs []string) ([]string, error) {
	digests, err := scanStrings(ctx, tx, `
		select rc.digest from run_steps s
		join run_content rc on rc.ref = s.payload->>'input_ref'
		where s.run_id = any($1::text[]) and s.seq = 1 and s.kind = 'run_started'
		union
		select rc.digest from regression_cases c
		join run_content rc on rc.ref = c.input_ref
		where c.from_run = any($1::text[])`, runs)
	if err != nil {
		return nil, fmt.Errorf("admin: read the input of erased runs: %w", err)
	}
	return digests, nil
}

func scanStrings(ctx context.Context, conn db, query string, args ...any) ([]string, error) {
	rows, err := conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
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
