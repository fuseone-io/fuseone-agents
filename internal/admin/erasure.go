package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// Content is the claim check erasure reaches, declared here by the consumer.
//
// Only the content. The ledger has no delete and this never asks for one: a
// correction to the record is a new step, and an erasure is not a correction —
// it is the payload going while the record of it happening stays.
type Content interface {
	Erase(ctx context.Context, owner string, reason string) (int, error)
	ErasePast(ctx context.Context, before time.Time, reason string) (int, error)
}

// Erasures performs and records the two ways content goes.
type Erasures struct {
	pool      *pgxpool.Pool
	content   Content
	retention *Retention
	clock     func() time.Time
}

func NewErasures(pool *pgxpool.Pool, content Content, retention *Retention) *Erasures {
	return &Erasures{
		pool: pool, content: content, retention: retention, clock: time.Now,
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
	erased := 0
	for _, run := range runs {
		count, err := e.content.Erase(ctx, string(run), "subject")
		if err != nil {
			return erased, fmt.Errorf("admin: erase %s: %w", run, err)
		}
		erased += count
	}

	// Recorded as one act, because that is what it was: a request about a
	// person, covering the runs the operator found. An erasure nobody can
	// account for is indistinguishable from data loss.
	if err := e.record(ctx, by, scope, "content.erased", map[string]any{
		"runs": len(runs), "objects": erased, "reason": reason,
	}); err != nil {
		return erased, err
	}
	return erased, nil
}

/*
Sweep erases content older than the configured window.

It reads the window every time rather than caching it: shortening retention
has to take effect on the next sweep, which is the whole reason it is a
setting and not a flag.
*/
func (e *Erasures) Sweep(ctx context.Context) (int, error) {
	window, err := e.retention.Window(ctx)
	if err != nil {
		return 0, err
	}
	// Never below what is configured (AU-11). The floor on the setting is
	// what keeps this from being an installation-wide delete, and reading it
	// back here is what keeps a corrupted row from becoming one.
	if window < MinRetention {
		return 0, fmt.Errorf("admin: refusing to sweep on a window of %s", window)
	}

	erased, err := e.content.ErasePast(ctx, e.clock().Add(-window), "retention")
	if err != nil {
		return 0, err
	}
	if erased == 0 {
		// Nothing aged out, which is the ordinary case. Recording it would
		// bury the sweeps that did something under thousands that did not.
		return 0, nil
	}

	return erased, e.record(ctx, "", domain.Scope{}, "content.expired", map[string]any{
		"objects": erased, "olderThanHours": int64(window / time.Hour),
	})
}

func (e *Erasures) record(
	ctx context.Context, by domain.UserID, scope domain.Scope, action string, detail any,
) error {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := Record(ctx, tx, Event{
		Principal: by, Scope: scope, Action: action, Target: "content", Detail: detail,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
