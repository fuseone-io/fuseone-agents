package ledger

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fuseone/agents/internal/domain"
)

/*
EventDispatchBacklog is what the event dispatcher has not reached (#228).

Bounded by construction, because a health page and a metrics loop read it
during the very incidents it describes — while xmin is stuck the range past
the cursor grows without limit, and a read that walked it would grow with
the incident. Each count stops at domain.EventDispatchCountCap, and the wait
is read from one row (see eventDispatchBacklogSQL). The split is the feed's
own boundary: below pg_snapshot_xmin the feed can read a run, at or above it
the run is committed and waits for the oldest transaction in flight.

A run's wait starts at its end, or at its last write when no end was
recorded: ended_at is nullable and 0092 positions a row inserted already
finished. updated_at is never null, so a run waiting is never read as a run
that has waited no time.
*/
func (p *Postgres) EventDispatchBacklog(ctx context.Context) (domain.EventDispatchBacklog, error) {
	cursor, err := p.EventCursor(ctx)
	if err != nil {
		return domain.EventDispatchBacklog{}, err
	}
	var out domain.EventDispatchBacklog
	var oldest *time.Time
	err = p.pool.QueryRow(ctx, eventDispatchBacklogSQL,
		strconv.FormatUint(cursor.Xid, 10), string(cursor.Run), domain.EventDispatchCountCap+1,
	).Scan(&out.Ready, &out.HeldByXmin, &oldest)
	if err != nil {
		return domain.EventDispatchBacklog{}, fmt.Errorf("read event dispatch backlog: %w", err)
	}
	if oldest != nil {
		out.OldestFinishedAt = oldest.UTC()
	}
	out = capBacklog(out)
	out.Xact, err = p.oldestXact(ctx, out.Held())
	return out, err
}

/*
oldestXact is the oldest open transaction that holds an id: the one
pg_snapshot_xmin is, in any database of the cluster. Read-only transactions
hold no id and so hold nothing back.

pg_stat_activity is readable by every role, but a role without
pg_read_all_stats (or membership of the session's role) sees another
session's backend_xid and not its xact_start. That reads as a holder of
unknown age. An installation that revoked the view entirely reads the same
when runs are held — they prove a holder is open — rather than failing the
health page over a monitoring grant.
*/
func (p *Postgres) oldestXact(ctx context.Context, held bool) (domain.OpenXact, error) {
	var started *time.Time
	err := p.pool.QueryRow(ctx, `
		select xact_start from pg_stat_activity
		where backend_xid is not null and pid <> pg_backend_pid()
		order by age(backend_xid) desc
		limit 1`).Scan(&started)
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return domain.OpenXact{}, nil
	case errors.As(err, &pgErr) && pgErr.Code == insufficientPrivilege:
		return domain.OpenXact{Open: held}, nil
	case err != nil:
		return domain.OpenXact{}, fmt.Errorf("read oldest open transaction: %w", err)
	case started == nil:
		return domain.OpenXact{Open: true}, nil
	}
	return domain.OpenXact{Open: true, StartedAt: started.UTC()}, nil
}

// capBacklog stops each count at the cap and says when it did; a count read
// with a limit of the cap plus one reached it when it came back above it.
func capBacklog(b domain.EventDispatchBacklog) domain.EventDispatchBacklog {
	if b.Ready > domain.EventDispatchCountCap {
		b.Ready, b.Capped = domain.EventDispatchCountCap, true
	}
	if b.HeldByXmin > domain.EventDispatchCountCap {
		b.HeldByXmin, b.Capped = domain.EventDispatchCountCap, true
	}
	return b
}

// insufficientPrivilege is SQLSTATE 42501.
const insufficientPrivilege = "42501"

/*
eventDispatchBacklogSQL reads three bounded things, each an index range on
runs_finished_xid_idx starting at the cursor ($1, $2):

  - the ready runs, below pg_snapshot_xmin, stopping after $3;
  - the held runs, at or above it, stopping after $3;
  - the first run past the cursor in the feed's order — one row — for the
    wait: the run the dispatcher will reach first is the one that has
    waited longest for it.

$3 is the count cap plus one, so a count of $3 says the cap was reached
without reading further. Sharing one statement shares one snapshot, so the
split is the feed's own boundary at one instant.
*/
const eventDispatchBacklogSQL = `
		select
		  (select count(*) from (
		     select 1 from runs
		     where finished_xid is not null
		       and (finished_xid, run_id) > ($1::xid8, $2)
		       and finished_xid < pg_snapshot_xmin(pg_current_snapshot())
		       and phase = 'finished' and ` + realRuns + `
		     order by finished_xid, run_id
		     limit $3) ready),
		  (select count(*) from (
		     select 1 from runs
		     where finished_xid >= pg_snapshot_xmin(pg_current_snapshot())
		       and (finished_xid, run_id) > ($1::xid8, $2)
		       and phase = 'finished' and ` + realRuns + `
		     order by finished_xid, run_id
		     limit $3) held),
		  (select coalesce(ended_at, updated_at) from runs
		     where finished_xid is not null
		       and (finished_xid, run_id) > ($1::xid8, $2)
		       and phase = 'finished' and ` + realRuns + `
		     order by finished_xid, run_id
		     limit 1)`
