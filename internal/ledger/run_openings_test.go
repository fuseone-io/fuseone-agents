package ledger_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/testdb"
)

/*
0126 makes one key open one run (#432). Two things it must get right about the runs already
there: a key two runs shared before it names the run that opened first, and a run that opened
while it ran is not left out of the openings it claims.
*/

func openedUnder(t *testing.T, store *ledger.Postgres, run domain.RunID, key string, at time.Time) {
	t.Helper()
	s := step(run, domain.StepRunStarted)
	s.IdemKey, s.At = key, at
	if _, err := store.Append(context.Background(), s); err != nil {
		t.Fatalf("open %s: %v", run, err)
	}
}

// Before 0126, two openings a millisecond apart under one key were both sealed. The key
// answers with the earlier, whichever was written first.
func TestRunOpenings_aKeyTwoRunsShared_namesTheOneThatOpenedFirst(t *testing.T) {
	pool := testdb.IsolatedThrough(t, "0125_case_worlds")
	store := ledger.NewPostgres(pool)
	at := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	openedUnder(t, store, "run-later", "hook:shared", at.Add(time.Millisecond))
	openedUnder(t, store, "run-earlier", "hook:shared", at)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if got, err := store.RunByIdemKey(t.Context(), "hook:shared"); err != nil || got != "run-earlier" {
		t.Errorf("the key names %s (%v), want run-earlier", got, err)
	}
}

/*
An opening that commits after the backfill's first pass read run_steps, and before the trigger
was in, claims no key itself: the second pass claims it. The migration is run here in its two
halves with that opening between them, which is the window a pod opening runs during the
migration has.
*/
func TestRunOpenings_anOpeningBetweenTheBackfillAndTheTrigger_isClaimed(t *testing.T) {
	pool := testdb.IsolatedThrough(t, "0125_case_worlds")
	ctx := t.Context()
	raw, err := os.ReadFile("sql/0126_run_openings.sql")
	if err != nil {
		t.Fatalf("read the migration: %v", err)
	}
	cut := strings.Index(string(raw), "create function claim_run_opening")
	if cut < 0 {
		t.Fatal("the migration no longer creates the trigger where this test cuts it")
	}
	if _, err := pool.Exec(ctx, string(raw[:cut])); err != nil {
		t.Fatalf("first half: %v", err)
	}
	openedUnder(t, ledger.NewPostgres(pool), "run-between", "hook:between", time.Now())
	if _, err := pool.Exec(ctx, string(raw[cut:])); err != nil {
		t.Fatalf("second half: %v", err)
	}
	var claimed string
	if err := pool.QueryRow(ctx, `select run_id from run_openings where idem_key = 'hook:between'`).Scan(&claimed); err != nil || claimed != "run-between" {
		t.Errorf("claimed %q (%v), want the opening that arrived between the two", claimed, err)
	}
}
