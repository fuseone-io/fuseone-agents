package ledger_test

import (
	"context"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/testdb"
)

/*
The feed reads only below pg_snapshot_xmin, and xmin is the cluster's, not
the database's: a transaction left open in any other database of the same
server — another suite, another developer's psql — holds every run these
tests finish behind it, for as long as it stays open. A test that assumed no
such transaction exists failed whenever one did.

So the feed tests assert against the boundary they observe. A read is
bracketed by the xmin seen just before and just after it; xmin only moves
forward, so the feed's own boundary lies between the two. Every run below the
first must be returned, none at or above the second may be, and what is
returned is a prefix of the runs in the feed's order. With nothing else open
— CI — the bracket sits above every run and the assertions are the full ones;
with a foreign transaction holding xmin they still hold, and say exactly what
the feed may return.
*/

// window is the xmin bracket around one read: lo before, hi after. Memory has
// no transaction in flight, so its window is above everything.
type window struct{ lo, hi uint64 }

var openWindow = window{lo: math.MaxUint64, hi: math.MaxUint64}

// rawPool is a pool on the database a Postgres store reads, or nil for the
// memory ledger.
func rawPool(t *testing.T, s Store) *pgxpool.Pool {
	t.Helper()
	if _, ok := s.(*ledger.Postgres); !ok {
		return nil
	}
	pool, err := pgxpool.New(context.Background(), testdb.DSN(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func xminNow(t *testing.T, pool *pgxpool.Pool) uint64 {
	t.Helper()
	var raw string
	if err := pool.QueryRow(context.Background(),
		`select pg_snapshot_xmin(pg_current_snapshot())::text`).Scan(&raw); err != nil {
		t.Fatalf("read xmin: %v", err)
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		t.Fatalf("xmin %q: %v", raw, err)
	}
	return n
}

// bracket runs read between two observations of xmin.
func bracket(t *testing.T, pool *pgxpool.Pool, read func()) window {
	t.Helper()
	if pool == nil {
		read()
		return openWindow
	}
	lo := xminNow(t, pool)
	read()
	return window{lo: lo, hi: xminNow(t, pool)}
}

/*
positionOf is where a finished run stands in the feed, read from the row
itself rather than from the feed — which may not offer it while a foreign
transaction holds xmin. The memory ledger has nothing in flight, so its feed
offers every run.
*/
func positionOf(t *testing.T, s Store, id domain.RunID) domain.FinishCursor {
	t.Helper()
	if pool := rawPool(t, s); pool != nil {
		var raw *string
		if err := pool.QueryRow(context.Background(),
			`select finished_xid::text from runs where run_id = $1`, string(id)).Scan(&raw); err != nil {
			t.Fatalf("position of %s: %v", id, err)
		}
		if raw == nil {
			t.Fatalf("%s has no position in the feed", id)
		}
		n, err := strconv.ParseUint(*raw, 10, 64)
		if err != nil {
			t.Fatalf("position of %s %q: %v", id, *raw, err)
		}
		return domain.FinishCursor{Xid: n, Run: id}
	}
	all, err := s.FinishedSince(context.Background(), domain.FinishCursor{}, 1000)
	if err != nil {
		t.Fatalf("FinishedSince: %v", err)
	}
	for _, r := range all {
		if r.Run.RunID == id {
			return r.At
		}
	}
	t.Fatalf("%s is not in the memory feed", id)
	return domain.FinishCursor{}
}

// settle gives xmin a bounded moment to pass the runs a test just finished,
// so that with nothing else open the assertions are the full ones. It never
// fails: a foreign transaction may hold xmin for longer than any test waits.
func settle(t *testing.T, s Store, want ...domain.FinishCursor) {
	t.Helper()
	pool := rawPool(t, s)
	if pool == nil || len(want) == 0 {
		return
	}
	top := uint64(0)
	for _, at := range want {
		top = max(top, at.Xid)
	}
	deadline := time.Now().Add(5 * time.Second)
	for xminNow(t, pool) <= top {
		if time.Now().After(deadline) {
			t.Logf("xmin is held below this test's runs by a transaction elsewhere on the cluster; asserting against the boundary observed")
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// readFeed reads one page of the feed and the xmin bracket around it.
func readFeed(t *testing.T, s Store, after domain.FinishCursor, limit int) ([]domain.FinishedRun, window) {
	t.Helper()
	var got []domain.FinishedRun
	var err error
	w := bracket(t, rawPool(t, s), func() { got, err = s.FinishedSince(context.Background(), after, limit) })
	if err != nil {
		t.Fatalf("FinishedSince: %v", err)
	}
	return got, w
}

/*
requireFeedPrefix asserts a page against the runs that may appear, in the
feed's order: the page is a prefix of them, holds every one below the
window's low edge, and none at or above its high edge — each up to limit.
*/
func requireFeedPrefix(t *testing.T, got []domain.FinishedRun, w window, want []domain.FinishCursor, limit int) {
	t.Helper()
	must, may := 0, 0
	for _, at := range want {
		if at.Xid < w.lo {
			must++
		}
		if at.Xid < w.hi {
			may++
		}
	}
	must, may = min(must, limit), min(may, limit)
	if len(got) < must || len(got) > may {
		t.Fatalf("page = %v; want between %d and %d of %v (xmin window %d..%d)",
			feedIDs(got), must, may, want, w.lo, w.hi)
	}
	for i, r := range got {
		if r.At != want[i] {
			t.Fatalf("page = %v; want a prefix of %v in the feed's order", feedIDs(got), want)
		}
	}
}
