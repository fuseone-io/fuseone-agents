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

Cheap by construction, because a health page and a metrics loop read it
during the very incidents it describes. The runs past the cursor are a range
of the partial index on (finished_xid, run_id), which holds only runs that
finished since migration 0092; a dispatcher that keeps up leaves that range
empty. The split is the feed's own boundary, in the same statement's
snapshot: below pg_snapshot_xmin the feed can read a run, at or above it the
run is committed and waits for the oldest transaction in flight.
*/
func (p *Postgres) EventDispatchBacklog(ctx context.Context) (domain.EventDispatchBacklog, error) {
	cursor, err := p.EventCursor(ctx)
	if err != nil {
		return domain.EventDispatchBacklog{}, err
	}
	var out domain.EventDispatchBacklog
	var oldest *time.Time
	err = p.pool.QueryRow(ctx, `
		with boundary as (select pg_snapshot_xmin(pg_current_snapshot()) as xmin)
		select count(*) filter (where finished_xid < boundary.xmin),
		       count(*) filter (where finished_xid >= boundary.xmin),
		       min(ended_at)
		from runs, boundary
		where finished_xid is not null
		  and (finished_xid, run_id) > ($1::xid8, $2)
		  and phase = 'finished' and `+realRuns,
		strconv.FormatUint(cursor.Xid, 10), string(cursor.Run),
	).Scan(&out.Ready, &out.HeldByXmin, &oldest)
	if err != nil {
		return domain.EventDispatchBacklog{}, fmt.Errorf("read event dispatch backlog: %w", err)
	}
	if oldest != nil {
		out.OldestFinishedAt = oldest.UTC()
	}
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

// insufficientPrivilege is SQLSTATE 42501.
const insufficientPrivilege = "42501"
