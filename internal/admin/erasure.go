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

	if err := e.recordSubjectErasure(ctx, by, scope, runs, erased, reason); err != nil {
		return erased, err
	}
	return erased, nil
}

func (e *Erasures) recordSubjectErasure(
	ctx context.Context, by domain.UserID, scope domain.Scope,
	runs []domain.RunID, erased int, reason string,
) error {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin subject erasure record: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	memoryRecords, err := e.markMemorySourcesErased(ctx, tx, runs, by, e.clock())
	if err != nil {
		return err
	}
	// Recorded as one act, because that is what it was: a request about a
	// person, covering the runs the operator found. An erasure nobody can
	// account for is indistinguishable from data loss.
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: scope, Action: "content.erased", Target: "content",
		Detail: map[string]any{
			"runs": len(runs), "objects": erased, "reason": reason,
			"memoryRecords": memoryRecords,
		},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
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

	before := e.clock().Add(-window)
	erased, err := e.content.ErasePast(ctx, before, "retention")
	if err != nil {
		return 0, err
	}

	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return erased, fmt.Errorf("admin: begin retention sweep: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	channelRecords, err := e.eraseChannelOperationalRows(ctx, tx, before)
	if err != nil {
		return erased, err
	}
	egressRecords, err := e.eraseEgressOperationalRows(ctx, tx, before)
	if err != nil {
		return erased, err
	}
	memoryRecords, err := e.eraseMemoryRows(ctx, tx, before)
	if err != nil {
		return erased, err
	}
	total := erased + channelRecords + egressRecords + memoryRecords
	if total == 0 {
		// Nothing aged out, which is the ordinary case. Recording it would
		// bury the sweeps that did something under thousands that did not.
		return 0, nil
	}

	if err := Record(ctx, tx, Event{
		Principal: "", Scope: domain.Scope{}, Action: "content.expired", Target: "content",
		Detail: map[string]any{
			"objects": erased, "channelRecords": channelRecords,
			"egressRecords":  egressRecords,
			"memoryRecords":  memoryRecords,
			"olderThanHours": int64(window / time.Hour),
		},
	}); err != nil {
		return erased, err
	}
	if err := tx.Commit(ctx); err != nil {
		return erased, fmt.Errorf("admin: commit retention sweep: %w", err)
	}
	return total, nil
}

const retentionChannelBatch = 5_000
const retentionEgressBatch = 5_000
const retentionMemoryBatch = 5_000

func (e *Erasures) eraseChannelOperationalRows(ctx context.Context, conn db, before time.Time) (int, error) {
	total := int64(0)
	for _, stmt := range []string{
		`with doomed as (
			select ctid from channel_inbox
			where at < $1
			  and not (
			    -- Owed replies are current work even when the original ask is
			    -- older than the retention window. A weekend approval must
			    -- still answer the Slack thread that asked.
			    (status = 'refused' and answered_at is null)
			    or (status = 'opened' and answer_due and answered_at is null and run_id <> '')
			  )
			order by at
			limit $2
		)
		delete from channel_inbox
		where ctid in (select ctid from doomed)`,
		`with doomed as (
			select ctid from channel_deliveries
			where posted_at < $1
			order by posted_at
			limit $2
		)
		delete from channel_deliveries
		where ctid in (select ctid from doomed)`,
		// last_seen, not first_seen: an incident that began long ago but is
		// still failing is current operational evidence, not expired history.
		`with doomed as (
			select ctid from channel_delivery_failures
			where last_seen < $1
			order by last_seen
			limit $2
		)
		delete from channel_delivery_failures
		where ctid in (select ctid from doomed)`,
	} {
		tag, err := conn.Exec(ctx, stmt, before.UTC(), retentionChannelBatch)
		if err != nil {
			return int(total), fmt.Errorf("admin: erase channel records: %w", err)
		}
		total += tag.RowsAffected()
	}
	return int(total), nil
}

func (e *Erasures) eraseEgressOperationalRows(ctx context.Context, conn db, before time.Time) (int, error) {
	// ctid is safe here because the rows are selected and deleted in one
	// statement. Do not split this into a read followed by a later delete.
	tag, err := conn.Exec(ctx, `
		with doomed as (
			select ctid from mcp_egress_denials
			where last_seen < $1
			order by last_seen
			limit $2
		)
		delete from mcp_egress_denials
		where ctid in (select ctid from doomed)`,
		before.UTC(), retentionEgressBatch)
	if err != nil {
		return 0, fmt.Errorf("admin: erase stdio egress records: %w", err)
	}
	return int(tag.RowsAffected()), nil
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
