package ledger

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
The finished-run feed the event dispatcher reads (#203, migration 0092).

Read in commit order, and only below the oldest transaction still in flight.
A run whose finish committed after a later one was read would otherwise land
behind the cursor for ever; below pg_snapshot_xmin every transaction has
committed or never will, so nothing can appear there later. A long-running
transaction elsewhere holds the feed back while it is open — it delays
dispatch, never skips it.
*/
func (p *Postgres) FinishedSince(
	ctx context.Context, after domain.FinishCursor, limit int,
) ([]domain.FinishedRun, error) {
	rows, err := p.pool.Query(ctx, `select`+runColumns+`, finished_xid::text
		from runs
		where finished_xid > '0'::xid8
		  and (finished_xid, run_id) > ($1::xid8, $2)
		  and finished_xid < pg_snapshot_xmin(pg_current_snapshot())
		  and phase = 'finished' and `+realRuns+`
		order by finished_xid, run_id
		limit $3`, strconv.FormatUint(after.Xid, 10), string(after.Run), limitOrDefault(limit))
	if err != nil {
		return nil, fmt.Errorf("read finished runs: %w", err)
	}
	defer rows.Close()

	var out []domain.FinishedRun
	for rows.Next() {
		var xid string
		summary, err := scanRunSummary(withTail{row: rows, tail: []any{&xid}})
		if err != nil {
			return nil, err
		}
		n, err := strconv.ParseUint(xid, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("finished run %s: transaction %q: %w", summary.RunID, xid, err)
		}
		out = append(out, domain.FinishedRun{Run: summary, At: domain.FinishCursor{Xid: n, Run: summary.RunID}})
	}
	return out, rows.Err()
}

// EventCursor is where the dispatcher has got to; the start when no replica
// has moved it yet.
func (p *Postgres) EventCursor(ctx context.Context) (domain.FinishCursor, error) {
	var xid, run string
	err := p.pool.QueryRow(ctx,
		`select finished_xid::text, run_id from event_dispatch_cursor`).Scan(&xid, &run)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FinishCursor{}, nil
	}
	if err != nil {
		return domain.FinishCursor{}, fmt.Errorf("read event cursor: %w", err)
	}
	n, err := strconv.ParseUint(xid, 10, 64)
	if err != nil {
		return domain.FinishCursor{}, fmt.Errorf("event cursor transaction %q: %w", xid, err)
	}
	return domain.FinishCursor{Xid: n, Run: domain.RunID(run)}, nil
}

// AdvanceEventCursor moves the cursor to to, unless it is already further.
// The comparison and the write are one statement, so two replicas finishing
// pages out of order keep the furthest; the earlier one is a no-op.
func (p *Postgres) AdvanceEventCursor(ctx context.Context, to domain.FinishCursor) error {
	_, err := p.pool.Exec(ctx, `
		insert into event_dispatch_cursor (finished_xid, run_id) values ($1::xid8, $2)
		on conflict (id) do update set finished_xid = excluded.finished_xid, run_id = excluded.run_id
		where (event_dispatch_cursor.finished_xid, event_dispatch_cursor.run_id)
		    < (excluded.finished_xid, excluded.run_id)`,
		strconv.FormatUint(to.Xid, 10), string(to.Run))
	if err != nil {
		return fmt.Errorf("advance event cursor: %w", err)
	}
	return nil
}

// withTail scans a row's leading columns into what the wrapped scanner asks
// for, and the columns after them into tail.
type withTail struct {
	row  rowScanner
	tail []any
}

func (w withTail) Scan(dest ...any) error {
	return w.row.Scan(append(dest, w.tail...)...)
}
