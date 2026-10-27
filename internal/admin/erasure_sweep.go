package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

const retentionChannelBatch = 5_000
const retentionEgressBatch = 5_000
const retentionMemoryBatch = 5_000

type sweepCounts struct {
	objects        int
	channelRecords int
	egressRecords  int
	memoryRecords  int
}

func (c sweepCounts) total() int {
	return c.objects + c.channelRecords + c.egressRecords + c.memoryRecords
}

/*
Sweep erases content older than the configured window.

It reads the window every time rather than caching it: shortening retention
has to take effect on the next sweep, which is the whole reason it is a
setting and not a flag.
*/
func (e *Erasures) Sweep(ctx context.Context) (int, error) {
	window, before, err := e.retentionBoundary(ctx)
	if err != nil {
		return 0, err
	}
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("admin: begin retention sweep: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	counts, err := e.eraseExpired(ctx, tx, before)
	if err != nil {
		return 0, err
	}
	if counts.total() == 0 {
		return 0, nil
	}
	if err := recordSweep(ctx, tx, counts, window); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("admin: commit retention sweep: %w", err)
	}
	return counts.total(), nil
}

func (e *Erasures) retentionBoundary(ctx context.Context) (time.Duration, time.Time, error) {
	window, err := e.retention.Window(ctx)
	if err != nil {
		return 0, time.Time{}, err
	}
	// Never below what is configured (AU-11). The floor on the setting is
	// what keeps this from becoming an installation-wide delete.
	if window < MinRetention {
		return 0, time.Time{}, fmt.Errorf("admin: refusing to sweep on a window of %s", window)
	}
	return window, e.clock().Add(-window), nil
}

func (e *Erasures) eraseExpired(
	ctx context.Context, tx pgx.Tx, before time.Time,
) (sweepCounts, error) {
	var counts sweepCounts
	var err error
	counts.objects, err = e.content.ErasePastTx(ctx, tx, before, "retention")
	if err != nil {
		return sweepCounts{}, err
	}
	counts.channelRecords, err = e.eraseChannelOperationalRows(ctx, tx, before)
	if err != nil {
		return sweepCounts{}, err
	}
	counts.egressRecords, err = e.eraseEgressOperationalRows(ctx, tx, before)
	if err != nil {
		return sweepCounts{}, err
	}
	counts.memoryRecords, err = e.eraseMemoryRows(ctx, tx, before)
	if err != nil {
		return sweepCounts{}, err
	}
	return counts, nil
}

func recordSweep(ctx context.Context, tx pgx.Tx, counts sweepCounts, window time.Duration) error {
	return Record(ctx, tx, Event{
		Principal: "", Scope: domain.Scope{}, Action: "content.expired", Target: "content",
		Detail: map[string]any{
			"objects": counts.objects, "channelRecords": counts.channelRecords,
			"egressRecords":  counts.egressRecords,
			"memoryRecords":  counts.memoryRecords,
			"olderThanHours": int64(window / time.Hour),
		},
	})
}

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
