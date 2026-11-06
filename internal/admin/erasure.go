package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// Content is the claim check erasure reaches, declared here by the consumer.
//
// Only the content. The ledger has no delete and this never asks for one: a
// correction to the record is a new step, and an erasure is not a correction —
// it is the payload going while the record of it happening stays.
type Content interface {
	EraseTx(ctx context.Context, tx pgx.Tx, kind, owner, reason string) (int, error)
	EraseRefsTx(ctx context.Context, tx pgx.Tx, refs []string, reason string) (int, error)
	ErasePastTx(ctx context.Context, tx pgx.Tx, before time.Time, reason string) (int, error)
}

// Erasures performs and records the two ways content goes.
type Erasures struct {
	pool      *pgxpool.Pool
	content   Content
	retention *Retention
	fence     CorpusFence
	clock     func() time.Time
}

// NewErasures needs the fence: without it, something copying a corpus could undo an
// erasure, so it is not something an installation can leave out.
func NewErasures(
	pool *pgxpool.Pool, content Content, retention *Retention, fence CorpusFence,
) *Erasures {
	return &Erasures{
		pool: pool, content: content, retention: retention, fence: fence, clock: time.Now,
	}
}

// WithClock makes a sweep assertable without waiting five years.
func (e *Erasures) WithClock(now func() time.Time) *Erasures {
	e.clock = now
	return e
}

/*
ForSubject erases what a set of runs was about, on somebody's request.

The runs are named by the caller rather than found here. Nothing in this
system indexes content by the person it concerns — deliberately, because an
index of who appears in what would be the very record a subject is asking to
be rid of. Finding the runs is the operator's act, through the trail, and
performing it is this one.
*/
func (e *Erasures) ForSubject(
	ctx context.Context, by domain.UserID, scope domain.Scope, runs []domain.RunID, reason string,
) (int, error) {
	named := memoryRunIDs(runs)
	agents, err := e.agentsOf(ctx, named)
	if err != nil {
		return 0, err
	}
	var erased int
	err = e.fence.Exclusive(ctx, agents, func(ctx context.Context) error {
		var err error
		erased, err = e.eraseSubject(ctx, subjectErasure{by, scope, runs, reason}, agents)
		return err
	})
	if err != nil {
		return 0, err
	}
	return erased, nil
}

type subjectErasure struct {
	by     domain.UserID
	scope  domain.Scope
	runs   []domain.RunID
	reason string
}

func (e *Erasures) eraseSubject(
	ctx context.Context, request subjectErasure, agents []domain.AgentID,
) (int, error) {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("admin: begin subject erasure: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	erased, err := e.eraseRuns(ctx, tx, request.runs)
	if err != nil {
		return 0, err
	}
	corpus, err := e.eraseCorpusOf(ctx, tx, memoryRunIDs(request.runs), agents)
	if err != nil {
		return 0, err
	}
	erased += corpus.objects
	memoryRecords, err := e.markMemorySourcesErased(ctx, tx, request.runs, request.by, e.clock())
	if err != nil {
		return 0, err
	}
	// Recorded as one act, because that is what it was: a request about a
	// person, covering the runs the operator found. An erasure nobody can
	// account for is indistinguishable from data loss.
	if err := Record(ctx, tx, Event{
		Principal: request.by, Scope: request.scope, Action: "content.erased", Target: "content",
		Detail: map[string]any{
			"runs": len(request.runs), "objects": erased, "reason": request.reason,
			"memoryRecords": memoryRecords, "corpusCases": corpus.cases,
		},
	}); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("admin: commit subject erasure: %w", err)
	}
	return erased, nil
}

func (e *Erasures) eraseRuns(ctx context.Context, tx pgx.Tx, runs []domain.RunID) (int, error) {
	erased := 0
	for _, run := range runs {
		count, err := e.content.EraseTx(ctx, tx, "run", string(run), "subject")
		if err != nil {
			return 0, fmt.Errorf("admin: erase %s: %w", run, err)
		}
		erased += count
	}
	return erased, nil
}
