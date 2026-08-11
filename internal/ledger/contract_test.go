package ledger_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
)

// One suite, both implementations.
//
// The in-memory ledger exists so tests are fast, not so they are easy. If it
// accepted anything Postgres rejects, every suite that used it would be
// certifying behaviour production does not have. These assertions run against
// both, and a divergence fails here rather than in an incident.

// Store is the surface the suite exercises. It is the union of what the engine
// and the API need.
type Store interface {
	engine.Ledger
	Runs(ctx context.Context) ([]domain.RunID, error)
	Verify(ctx context.Context, runID domain.RunID) error
	Claim(ctx context.Context, owner string, lease time.Duration) (domain.Claim, error)
	Release(ctx context.Context, runID domain.RunID, outcome domain.ClaimOutcome) error
	Stats(ctx context.Context, filter domain.RunFilter) (domain.RunStats, error)
	ListRuns(ctx context.Context, filter domain.RunFilter, phase string, limit int) ([]domain.RunSummary, error)
	CostRollup(ctx context.Context, filter domain.RunFilter, groupBy string) ([]domain.CostBucket, error)
	AgentActivity(ctx context.Context, filter domain.RunFilter) ([]domain.AgentActivity, error)
	Throughput(ctx context.Context, filter domain.RunFilter) ([]domain.ThroughputBucket, error)
	SpentSince(ctx context.Context, scope domain.Scope, since time.Time) (domain.Consumption, error)
}

type factory struct {
	name string
	open func(t *testing.T) Store
}

func implementations(t *testing.T) []factory {
	t.Helper()

	impls := []factory{{
		name: "memory",
		open: func(*testing.T) Store { return ledger.NewMemory() },
	}}

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		// Skipped rather than silently absent: a suite that quietly halves
		// itself on a laptop is how divergence gets to production.
		requireDatabase(t, dsn)
		t.Log("TEST_DATABASE_URL is unset; skipping the Postgres implementation")
		return impls
	}

	impls = append(impls, factory{
		name: "postgres",
		open: func(t *testing.T) Store { return openPostgres(t, dsn) },
	})
	return impls
}

func openPostgres(t *testing.T, dsn string) Store {
	t.Helper()
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := ledger.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Each test gets a clean ledger; these tables are the whole state.
	if _, err := pool.Exec(ctx, `truncate run_steps, runs`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return ledger.NewPostgres(pool)
}

// run executes fn against every implementation, so a test is written once.
func run(t *testing.T, name string, fn func(t *testing.T, store Store)) {
	t.Helper()
	for _, impl := range implementations(t) {
		t.Run(fmt.Sprintf("%s/%s", impl.name, name), func(t *testing.T) {
			fn(t, impl.open(t))
		})
	}
}

func step(runID domain.RunID, kind domain.StepKind) domain.Step {
	return domain.Step{
		RunID:      runID,
		Kind:       kind,
		Scope:      domain.Scope{Company: "acme", Area: "cx"},
		AgentID:    "triage",
		VersionID:  "v3",
		OnBehalfOf: "ana",
		At:         time.Now(),
	}
}

func TestContract(t *testing.T) {
	run(t, "appended steps form a verifiable chain", func(t *testing.T, s Store) {
		ctx := context.Background()

		for _, k := range []domain.StepKind{
			domain.StepRunStarted, domain.StepPlanned,
			domain.StepGateDecided, domain.StepToolCalled,
		} {
			if _, err := s.Append(ctx, step("run-1", k)); err != nil {
				t.Fatalf("Append(%s): %v", k, err)
			}
		}

		if err := s.Verify(ctx, "run-1"); err != nil {
			t.Fatalf("Verify: %v", err)
		}
	})

	run(t, "a payload survives storage byte-for-byte as far as the hash is concerned",
		func(t *testing.T, s Store) {
			ctx := context.Background()

			// Written with keys out of alphabetical order and with whitespace.
			// A store that reorders or reformats JSON — jsonb does both — must
			// still return something that verifies.
			original := step("run-1", domain.StepGateDecided)
			original.Payload = []byte(`{ "zebra": 1, "alpha": "x", "middle": [3, 2, 1] }`)

			if _, err := s.Append(ctx, original); err != nil {
				t.Fatalf("Append: %v", err)
			}

			steps, err := s.Read(ctx, "run-1", domain.FirstSeq)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if err := domain.VerifyChain(steps); err != nil {
				t.Fatalf("chain broken after a storage round trip: %v", err)
			}
		})

	run(t, "the same idempotency key is refused a second time", func(t *testing.T, s Store) {
		ctx := context.Background()

		call := step("run-1", domain.StepToolCalled)
		call.IdemKey = "run-1:crm.refund:9f2a"

		if _, err := s.Append(ctx, call); err != nil {
			t.Fatalf("first Append: %v", err)
		}
		if _, err := s.Append(ctx, call); !errors.Is(err, ledger.ErrIdemConflict) {
			t.Errorf("second Append = %v, want %v", err, ledger.ErrIdemConflict)
		}
	})

	run(t, "concurrent writers produce a contiguous chain with no gaps", func(t *testing.T, s Store) {
		ctx := context.Background()

		const writers = 16
		var wg sync.WaitGroup
		wg.Add(writers)
		for range writers {
			go func() {
				defer wg.Done()
				_, _ = s.Append(ctx, step("run-1", domain.StepPlanned))
			}()
		}
		wg.Wait()

		steps, err := s.Read(ctx, "run-1", domain.FirstSeq)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(steps) != writers {
			t.Fatalf("stored %d steps, want %d", len(steps), writers)
		}
		for i, st := range steps {
			if want := int64(i + 1); st.Seq != want {
				t.Fatalf("steps[%d].Seq = %d, want %d — gap or duplicate", i, st.Seq, want)
			}
		}
		if err := domain.VerifyChain(steps); err != nil {
			t.Errorf("VerifyChain after concurrent append: %v", err)
		}
	})

	run(t, "reading from mid-chain returns only the remainder", func(t *testing.T, s Store) {
		ctx := context.Background()
		for range 5 {
			if _, err := s.Append(ctx, step("run-1", domain.StepPlanned)); err != nil {
				t.Fatalf("Append: %v", err)
			}
		}

		steps, err := s.Read(ctx, "run-1", 4)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(steps) != 2 || steps[0].Seq != 4 {
			t.Errorf("got %d steps starting at %d, want 2 starting at 4", len(steps), steps[0].Seq)
		}
	})

	run(t, "an unknown run is reported as not found", func(t *testing.T, s Store) {
		if _, err := s.Read(context.Background(), "nope", domain.FirstSeq); !errors.Is(err, ledger.ErrNotFound) {
			t.Errorf("Read = %v, want %v", err, ledger.ErrNotFound)
		}
	})

	run(t, "runs are listed newest first", func(t *testing.T, s Store) {
		ctx := context.Background()

		older := step("run-old", domain.StepRunStarted)
		older.At = time.Now().Add(-time.Hour)
		newer := step("run-new", domain.StepRunStarted)

		for _, st := range []domain.Step{older, newer} {
			if _, err := s.Append(ctx, st); err != nil {
				t.Fatalf("Append: %v", err)
			}
		}

		ids, err := s.Runs(ctx)
		if err != nil {
			t.Fatalf("Runs: %v", err)
		}
		if len(ids) != 2 || ids[0] != "run-new" {
			t.Errorf("Runs = %v, want run-new first", ids)
		}
	})

	// The projection is an optimisation, and an optimisation that disagrees
	// with the ledger is worse than no optimisation. Whatever a store reports
	// must equal what a fold of its own steps produces.
	run(t, "the store agrees with a fold of its own ledger", func(t *testing.T, s Store) {
		ctx := context.Background()

		reserve := step("run-1", domain.StepBudgetReserved)
		reserve.Payload = []byte(`{"micros":50000}`)
		returned := step("run-1", domain.StepToolReturned)
		returned.Labels = domain.NewLabels(domain.LabelUntrusted)
		returned.Cost = domain.Cost{InputTokens: 1200, Micros: 9_000}

		for _, st := range []domain.Step{
			step("run-1", domain.StepRunStarted),
			step("run-1", domain.StepPlanned),
			reserve,
			step("run-1", domain.StepToolCalled),
			returned,
		} {
			if _, err := s.Append(ctx, st); err != nil {
				t.Fatalf("Append(%s): %v", st.Kind, err)
			}
		}

		steps, err := s.Read(ctx, "run-1", domain.FirstSeq)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		state, err := engine.Fold(steps)
		if err != nil {
			t.Fatalf("Fold: %v", err)
		}

		if state.Phase != engine.PhaseRunning {
			t.Errorf("Phase = %v, want running", state.Phase)
		}
		if state.Reserved.Micros != 50_000 {
			t.Errorf("Reserved.Micros = %d, want 50000", state.Reserved.Micros)
		}
		if !state.Labels.Has(domain.LabelUntrusted) {
			t.Errorf("Labels = %v, want the untrusted label to have propagated", state.Labels)
		}
		if state.Spent.ToolCalls != 1 {
			t.Errorf("ToolCalls = %d, want 1", state.Spent.ToolCalls)
		}
	})
}

// The work queue. A worker pool is only safe if two workers cannot hold the
// same run, and only useful if a worker that dies releases what it held.
func TestQueueContract(t *testing.T) {
	const lease = time.Minute

	// openRun leaves a run in a claimable phase.
	openRun := func(t *testing.T, s Store, id domain.RunID) {
		t.Helper()
		if _, err := s.Append(context.Background(), step(id, domain.StepRunStarted)); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	run(t, "an empty queue reports no work rather than failing", func(t *testing.T, s Store) {
		_, err := s.Claim(context.Background(), "w1", lease)
		if !errors.Is(err, domain.ErrNoClaimableRun) {
			t.Errorf("Claim = %v, want %v", err, domain.ErrNoClaimableRun)
		}
	})

	run(t, "a claimed run is not handed to a second worker", func(t *testing.T, s Store) {
		ctx := context.Background()
		openRun(t, s, "run-1")

		first, err := s.Claim(ctx, "w1", lease)
		if err != nil {
			t.Fatalf("first Claim: %v", err)
		}
		if first.RunID != "run-1" {
			t.Fatalf("claimed %q, want run-1", first.RunID)
		}

		// Two workers advancing the same run would duplicate every turn.
		if _, err := s.Claim(ctx, "w2", lease); !errors.Is(err, domain.ErrNoClaimableRun) {
			t.Errorf("second Claim = %v, want %v", err, domain.ErrNoClaimableRun)
		}
	})

	run(t, "a released run is claimable again", func(t *testing.T, s Store) {
		ctx := context.Background()
		openRun(t, s, "run-1")

		if _, err := s.Claim(ctx, "w1", lease); err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if err := s.Release(ctx, "run-1", domain.ClaimOutcome{}); err != nil {
			t.Fatalf("Release: %v", err)
		}
		if _, err := s.Claim(ctx, "w2", lease); err != nil {
			t.Errorf("Claim after release = %v, want a claim", err)
		}
	})

	run(t, "the claim carries what the worker needs to advance", func(t *testing.T, s Store) {
		ctx := context.Background()
		openRun(t, s, "run-1")

		c, err := s.Claim(ctx, "w1", lease)
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		// A worker holds no memory of a run between turns, so everything it
		// needs to resolve the spec has to come back with the claim.
		if c.AgentID != "triage" || c.VersionID != "v3" || c.Scope.Company != "acme" {
			t.Errorf("claim = %+v, want agent triage@v3 in acme", c)
		}
	})

	run(t, "consecutive failures accumulate and progress resets them", func(t *testing.T, s Store) {
		ctx := context.Background()
		openRun(t, s, "run-1")

		boom := domain.ClaimOutcome{Err: errors.New("upstream refused")}
		for want := range 2 {
			c, err := s.Claim(ctx, "w1", lease)
			if err != nil {
				t.Fatalf("Claim %d: %v", want, err)
			}
			if c.Attempts != want {
				t.Fatalf("Attempts = %d on claim %d, want %d", c.Attempts, want, want)
			}
			if err := s.Release(ctx, "run-1", boom); err != nil {
				t.Fatalf("Release: %v", err)
			}
		}

		// A turn that makes progress clears the count: the threshold measures
		// consecutive failures, not failures ever.
		if _, err := s.Claim(ctx, "w1", lease); err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if err := s.Release(ctx, "run-1", domain.ClaimOutcome{}); err != nil {
			t.Fatalf("Release: %v", err)
		}
		c, err := s.Claim(ctx, "w1", lease)
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if c.Attempts != 0 {
			t.Errorf("Attempts = %d after a successful turn, want 0", c.Attempts)
		}
	})

	run(t, "a backoff keeps the run out of the queue until it is due", func(t *testing.T, s Store) {
		ctx := context.Background()
		openRun(t, s, "run-1")

		if _, err := s.Claim(ctx, "w1", lease); err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if err := s.Release(ctx, "run-1", domain.ClaimOutcome{
			Err:           errors.New("upstream refused"),
			NextAttemptAt: time.Now().Add(time.Hour),
		}); err != nil {
			t.Fatalf("Release: %v", err)
		}

		if _, err := s.Claim(ctx, "w1", lease); !errors.Is(err, domain.ErrNoClaimableRun) {
			t.Errorf("Claim during backoff = %v, want %v", err, domain.ErrNoClaimableRun)
		}
	})

	run(t, "a parked run stays out of the queue until a human acts", func(t *testing.T, s Store) {
		ctx := context.Background()
		openRun(t, s, "run-1")

		if _, err := s.Claim(ctx, "w1", lease); err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if err := s.Release(ctx, "run-1", domain.ClaimOutcome{
			Err: errors.New("gave up"), Parked: true,
		}); err != nil {
			t.Fatalf("Release: %v", err)
		}

		// Parking exists because retrying will not help. A worker that picked
		// it up anyway would turn the supervision policy into an infinite loop.
		if _, err := s.Claim(ctx, "w1", lease); !errors.Is(err, domain.ErrNoClaimableRun) {
			t.Errorf("Claim on a parked run = %v, want %v", err, domain.ErrNoClaimableRun)
		}
	})

	run(t, "an expired lease returns the run to the queue", func(t *testing.T, s Store) {
		ctx := context.Background()
		openRun(t, s, "run-1")

		// A worker that dies never releases. Nothing may depend on it doing so.
		if _, err := s.Claim(ctx, "w1", -time.Minute); err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if _, err := s.Claim(ctx, "w2", lease); err != nil {
			t.Errorf("Claim after lease expiry = %v, want a claim", err)
		}
	})

	run(t, "a run awaiting a human is never claimed", func(t *testing.T, s Store) {
		ctx := context.Background()
		openRun(t, s, "run-1")

		req := step("run-1", domain.StepApprovalRequested)
		req.Payload = []byte(`{"tool":"crm.note","rule":"taint"}`)
		if _, err := s.Append(ctx, req); err != nil {
			t.Fatalf("Append: %v", err)
		}

		if _, err := s.Claim(ctx, "w1", lease); !errors.Is(err, domain.ErrNoClaimableRun) {
			t.Errorf("Claim = %v, want %v — a suspended run is not work", err, domain.ErrNoClaimableRun)
		}
	})
}

// --- aggregates ------------------------------------------------------------

// startedAt seeds a run that began at a known instant, so a duration is a
// fact rather than whatever the clock did during the test.
func startedAt(runID domain.RunID, at time.Time) domain.Step {
	st := step(runID, domain.StepRunStarted)
	st.At = at
	return st
}

func finishedAt(runID domain.RunID, at time.Time) domain.Step {
	st := step(runID, domain.StepRunFinished)
	st.At = at
	return st
}

func TestStatsContract(t *testing.T) {
	base := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	run(t, "an empty ledger reports nothing rather than failing", func(t *testing.T, s Store) {
		got, err := s.Stats(context.Background(), domain.RunFilter{})
		if err != nil {
			t.Fatalf("Stats: %v", err)
		}
		if got.Total != 0 || len(got.ByPhase) != 0 {
			t.Errorf("Stats = %+v, want an empty tally", got)
		}
	})

	run(t, "runs are counted by the phase they are in", func(t *testing.T, s Store) {
		ctx := context.Background()

		mustAppend(t, s, startedAt("run-a", base))
		mustAppend(t, s, finishedAt("run-a", base.Add(2*time.Minute)))
		mustAppend(t, s, startedAt("run-b", base))
		mustAppend(t, s, startedAt("run-c", base))
		mustAppend(t, s, step("run-c", domain.StepParked))

		got, err := s.Stats(ctx, domain.RunFilter{})
		if err != nil {
			t.Fatalf("Stats: %v", err)
		}
		if got.Total != 3 {
			t.Errorf("Total = %d, want 3", got.Total)
		}
		for phase, want := range map[string]int64{"finished": 1, "running": 1, "parked": 1} {
			if got.Count(phase) != want {
				t.Errorf("%s = %d, want %d (got %+v)", phase, got.Count(phase), want, got.ByPhase)
			}
		}
	})

	run(t, "the median covers runs that ended, and says how many", func(t *testing.T, s Store) {
		ctx := context.Background()

		for i, minutes := range []int{1, 3, 11} {
			id := domain.RunID(fmt.Sprintf("run-%d", i))
			mustAppend(t, s, startedAt(id, base))
			mustAppend(t, s, finishedAt(id, base.Add(time.Duration(minutes)*time.Minute)))
		}
		// Still running: it must not drag the median of what has finished.
		mustAppend(t, s, startedAt("run-open", base))

		got, err := s.Stats(ctx, domain.RunFilter{})
		if err != nil {
			t.Fatalf("Stats: %v", err)
		}
		if got.Ended != 3 {
			t.Fatalf("Ended = %d, want 3", got.Ended)
		}
		if want := int64(3 * 60 * 1000); got.MedianDurationMS != want {
			t.Errorf("MedianDurationMS = %d, want %d", got.MedianDurationMS, want)
		}
	})

	run(t, "a since bound excludes runs that started before it", func(t *testing.T, s Store) {
		ctx := context.Background()

		mustAppend(t, s, startedAt("run-old", base.Add(-48*time.Hour)))
		mustAppend(t, s, startedAt("run-new", base))

		got, err := s.Stats(ctx, domain.RunFilter{Since: base.Add(-time.Hour)})
		if err != nil {
			t.Fatalf("Stats: %v", err)
		}
		if got.Total != 1 {
			t.Errorf("Total = %d, want only the recent run", got.Total)
		}
	})

	run(t, "an even number of runs gives the same median in both stores", func(t *testing.T, s Store) {
		ctx := context.Background()

		// Every fixture had an odd count, so nobody noticed that one store
		// averaged the two middle values and the other took the lower one.
		// The same runs gave different medians depending on who answered.
		for i, minutes := range []int{1, 2, 3, 4} {
			id := domain.RunID(fmt.Sprintf("run-%d", i))
			mustAppend(t, s, startedAt(id, base))
			mustAppend(t, s, finishedAt(id, base.Add(time.Duration(minutes)*time.Minute)))
		}

		got, err := s.Stats(ctx, domain.RunFilter{})
		if err != nil {
			t.Fatalf("Stats: %v", err)
		}
		// A duration some run actually had, never the average of two.
		if want := int64(2 * 60 * 1000); got.MedianDurationMS != want {
			t.Errorf("MedianDurationMS = %d, want %d", got.MedianDurationMS, want)
		}
	})

	run(t, "an until bound excludes runs that started after it", func(t *testing.T, s Store) {
		ctx := context.Background()

		// The filter carried Until and only the list honoured it, so every
		// figure meant to compare one window against another — yesterday
		// against today — silently counted both.
		mustAppend(t, s, startedAt("run-yesterday", base.Add(-24*time.Hour)))
		mustAppend(t, s, startedAt("run-today", base))

		got, err := s.Stats(ctx, domain.RunFilter{
			Since: base.Add(-36 * time.Hour),
			Until: base.Add(-12 * time.Hour),
		})
		if err != nil {
			t.Fatalf("Stats: %v", err)
		}
		if got.Total != 1 {
			t.Errorf("Total = %d, want only the run inside the window", got.Total)
		}
	})

	run(t, "the slow tail is reported next to the median, over the same runs", func(t *testing.T, s Store) {
		ctx := context.Background()

		// A median alone says nothing about the runs people complain about.
		for i, minutes := range []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 40} {
			id := domain.RunID(fmt.Sprintf("run-%d", i))
			mustAppend(t, s, startedAt(id, base))
			mustAppend(t, s, finishedAt(id, base.Add(time.Duration(minutes)*time.Minute)))
		}

		got, err := s.Stats(ctx, domain.RunFilter{})
		if err != nil {
			t.Fatalf("Stats: %v", err)
		}
		if got.MedianDurationMS != 60_000 {
			t.Errorf("MedianDurationMS = %d, want 60000", got.MedianDurationMS)
		}
		if want := int64(40 * 60 * 1000); got.P95DurationMS != want {
			t.Errorf("P95DurationMS = %d, want %d — the slow run is the point", got.P95DurationMS, want)
		}
	})

	run(t, "a scope bound counts only that area", func(t *testing.T, s Store) {
		ctx := context.Background()

		mine := startedAt("run-cx", base)
		theirs := startedAt("run-mkt", base)
		theirs.Scope = domain.Scope{Company: "acme", Area: "marketing"}

		mustAppend(t, s, mine)
		mustAppend(t, s, theirs)

		got, err := s.Stats(ctx, domain.RunFilter{Scope: domain.Scope{Company: "acme", Area: "cx"}})
		if err != nil {
			t.Fatalf("Stats: %v", err)
		}
		if got.Total != 1 {
			t.Errorf("Total = %d, want only the run in cx", got.Total)
		}
	})
}

func mustAppend(t *testing.T, s Store, st domain.Step) {
	t.Helper()
	if _, err := s.Append(context.Background(), st); err != nil {
		t.Fatalf("Append(%s): %v", st.Kind, err)
	}
}

// --- listing and rollup ----------------------------------------------------

func TestListContract(t *testing.T) {
	base := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	run(t, "a page carries what a list shows, without folding the ledger", func(t *testing.T, s Store) {
		ctx := context.Background()

		opened := startedAt("run-1", base)
		mustAppend(t, s, opened)

		priced := step("run-1", domain.StepPlanned)
		priced.At = base.Add(time.Second)
		priced.Cost = domain.Cost{InputTokens: 1200, OutputTokens: 90, CacheReadTokens: 400, Micros: 9_000}
		mustAppend(t, s, priced)
		mustAppend(t, s, finishedAt("run-1", base.Add(2*time.Minute)))

		page, err := s.ListRuns(ctx, domain.RunFilter{}, "", 50)
		if err != nil {
			t.Fatalf("ListRuns: %v", err)
		}
		if len(page) != 1 {
			t.Fatalf("ListRuns = %d runs, want 1", len(page))
		}

		got := page[0]
		if got.AgentID != opened.AgentID || got.Scope != opened.Scope {
			t.Errorf("identity = %s in %s, want %s in %s", got.AgentID, got.Scope, opened.AgentID, opened.Scope)
		}
		if got.Phase != "finished" || got.EndedAt.IsZero() {
			t.Errorf("phase = %q ended = %v, want a finished run with an end", got.Phase, got.EndedAt)
		}
		// The breakdown is the point: a total alone bills a run without
		// explaining it, and a cache read costs a fraction of an input token.
		if got.Cost.CacheReadTokens != 400 || got.Cost.InputTokens != 1200 || got.Cost.Micros != 9_000 {
			t.Errorf("cost = %+v, want the breakdown the steps carried", got.Cost)
		}
	})

	run(t, "the newest run comes first", func(t *testing.T, s Store) {
		mustAppend(t, s, startedAt("run-old", base.Add(-time.Hour)))
		mustAppend(t, s, startedAt("run-new", base))

		page, err := s.ListRuns(context.Background(), domain.RunFilter{}, "", 50)
		if err != nil {
			t.Fatalf("ListRuns: %v", err)
		}
		if len(page) != 2 || page[0].RunID != "run-new" {
			t.Errorf("ListRuns = %v, want run-new first", kindsOfPage(page))
		}
	})

	run(t, "the filter is applied where the whole set is, not on the page", func(t *testing.T, s Store) {
		ctx := context.Background()

		mustAppend(t, s, startedAt("run-open", base))
		mustAppend(t, s, startedAt("run-done", base))
		mustAppend(t, s, finishedAt("run-done", base.Add(time.Minute)))

		// A phase filter that ran after the page was cut would return one run
		// or none depending on how many rows the caller happened to ask for.
		page, err := s.ListRuns(ctx, domain.RunFilter{}, "finished", 1)
		if err != nil {
			t.Fatalf("ListRuns: %v", err)
		}
		if len(page) != 1 || page[0].RunID != "run-done" {
			t.Errorf("ListRuns = %v, want only the finished run", kindsOfPage(page))
		}
	})

	run(t, "a run waiting on a person carries what it is waiting for", func(t *testing.T, s Store) {
		ctx := context.Background()

		mustAppend(t, s, startedAt("run-1", base))
		asked := step("run-1", domain.StepApprovalRequested)
		asked.At = base.Add(time.Second)
		asked.Payload = []byte(`{"tool":"crm.note","rule":"taint","reason":"untrusted argument"}`)
		mustAppend(t, s, asked)

		page, err := s.ListRuns(ctx, domain.RunFilter{}, "awaiting_approval", 50)
		if err != nil {
			t.Fatalf("ListRuns: %v", err)
		}
		if len(page) != 1 || page[0].PendingApproval == nil {
			t.Fatalf("ListRuns = %v, want the suspended action denormalised", page)
		}
		if page[0].PendingApproval.Tool != "crm.note" {
			t.Errorf("pending tool = %q, want crm.note", page[0].PendingApproval.Tool)
		}
	})

	run(t, "cost sums by the dimension asked for", func(t *testing.T, s Store) {
		ctx := context.Background()

		for _, id := range []domain.RunID{"run-a", "run-b"} {
			mustAppend(t, s, startedAt(id, base))
			priced := step(id, domain.StepPlanned)
			priced.At = base.Add(time.Second)
			priced.Cost = domain.Cost{InputTokens: 100, Micros: 5_000}
			mustAppend(t, s, priced)
		}

		buckets, err := s.CostRollup(ctx, domain.RunFilter{Until: base.Add(time.Hour)}, "agent")
		if err != nil {
			t.Fatalf("CostRollup: %v", err)
		}
		if len(buckets) != 1 {
			t.Fatalf("CostRollup = %v, want one bucket; both runs share an agent", buckets)
		}
		if buckets[0].Runs != 2 || buckets[0].Cost.Micros != 10_000 {
			t.Errorf("bucket = %+v, want 2 runs totalling 10000 micros", buckets[0])
		}
		if buckets[0].Cost.InputTokens != 200 {
			t.Errorf("inputTokens = %d, want the breakdown summed too", buckets[0].Cost.InputTokens)
		}
	})

	run(t, "a rollup without an upper bound is refused", func(t *testing.T, s Store) {
		// A total that moves while somebody reads it is not a total. The bound
		// is what makes two people comparing the same figure see the same one.
		if _, err := s.CostRollup(context.Background(), domain.RunFilter{}, "agent"); err == nil {
			t.Error("CostRollup accepted a window with no end")
		}
	})

	run(t, "an unknown grouping is refused rather than guessed", func(t *testing.T, s Store) {
		// The dimension names a column, and a column name cannot be a bound
		// parameter — so the set is closed rather than whatever arrives.
		_, err := s.CostRollup(context.Background(),
			domain.RunFilter{Until: base}, "agent_id; drop table runs")
		if err == nil {
			t.Error("CostRollup accepted an arbitrary grouping")
		}
	})
}

func kindsOfPage(page []domain.RunSummary) []domain.RunID {
	out := make([]domain.RunID, len(page))
	for i, r := range page {
		out[i] = r.RunID
	}
	return out
}

// requireDatabase turns the skip into a failure where a skip would be a lie.
//
// These suites are the only place the in-memory ledger is checked against the
// real one, and they skip silently when TEST_DATABASE_URL is unset. That is
// right on a laptop and wrong in CI, where a mistyped variable would quietly
// halve the suite for as long as nobody noticed.
func requireDatabase(t *testing.T, dsn string) {
	t.Helper()
	if dsn == "" && os.Getenv("REQUIRE_DATABASE") != "" {
		t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
	}
}

func TestSearchContract(t *testing.T) {
	base := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	seed := func(t *testing.T, s Store) {
		t.Helper()
		mustAppend(t, s, startedAt("run-triage-8801", base))

		other := startedAt("run-leads-4410", base)
		other.AgentID = "lead-qualifier"
		mustAppend(t, s, other)
	}

	run(t, "a search matches the run identifier", func(t *testing.T, s Store) {
		seed(t, s)

		page, err := s.ListRuns(context.Background(), domain.RunFilter{Search: "8801"}, "", 50)
		if err != nil {
			t.Fatalf("ListRuns: %v", err)
		}
		if len(page) != 1 || page[0].RunID != "run-triage-8801" {
			t.Errorf("ListRuns = %v, want the run whose id matched", kindsOfPage(page))
		}
	})

	run(t, "a search matches the agent identifier", func(t *testing.T, s Store) {
		seed(t, s)

		page, err := s.ListRuns(context.Background(), domain.RunFilter{Search: "lead"}, "", 50)
		if err != nil {
			t.Fatalf("ListRuns: %v", err)
		}
		if len(page) != 1 || page[0].AgentID != "lead-qualifier" {
			t.Errorf("ListRuns = %v, want the run whose agent matched", kindsOfPage(page))
		}
	})

	run(t, "case does not decide whether something is found", func(t *testing.T, s Store) {
		seed(t, s)

		// Somebody typing a run id from a ticket should not have to match the
		// casing the platform happened to generate.
		page, err := s.ListRuns(context.Background(), domain.RunFilter{Search: "TRIAGE"}, "", 50)
		if err != nil {
			t.Fatalf("ListRuns: %v", err)
		}
		if len(page) != 1 {
			t.Errorf("ListRuns = %v, want a case-insensitive match", kindsOfPage(page))
		}
	})

	run(t, "a search that looks like SQL is a search", func(t *testing.T, s Store) {
		seed(t, s)

		page, err := s.ListRuns(context.Background(),
			domain.RunFilter{Search: "' or 1=1 --"}, "", 50)
		if err != nil {
			t.Fatalf("ListRuns: %v", err)
		}
		if len(page) != 0 {
			t.Errorf("ListRuns = %v, want nothing; the pattern is a bound parameter", kindsOfPage(page))
		}
	})
}

func TestAgentActivityContract(t *testing.T) {
	base := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	run(t, "an agent that never ran is absent rather than reported as idle", func(t *testing.T, s Store) {
		activity, err := s.AgentActivity(context.Background(), domain.RunFilter{})
		if err != nil {
			t.Fatalf("AgentActivity: %v", err)
		}
		if len(activity) != 0 {
			t.Errorf("AgentActivity = %v, want nothing; no runs exist", activity)
		}
	})

	run(t, "runs are counted, and the ones waiting on a person separately", func(t *testing.T, s Store) {
		ctx := context.Background()

		mustAppend(t, s, startedAt("run-a", base))
		mustAppend(t, s, finishedAt("run-a", base.Add(time.Minute)))
		mustAppend(t, s, startedAt("run-b", base))
		mustAppend(t, s, step("run-b", domain.StepParked))
		mustAppend(t, s, startedAt("run-c", base))

		activity, err := s.AgentActivity(ctx, domain.RunFilter{})
		if err != nil {
			t.Fatalf("AgentActivity: %v", err)
		}
		if len(activity) != 1 {
			t.Fatalf("AgentActivity = %d agents, want 1", len(activity))
		}

		got := activity[0]
		if got.Runs != 3 || got.Finished != 1 || got.Waiting != 1 {
			t.Errorf("activity = %+v, want 3 runs, 1 finished, 1 waiting", got)
		}
	})

	run(t, "the phase shown is the most recent run's", func(t *testing.T, s Store) {
		ctx := context.Background()

		mustAppend(t, s, startedAt("run-old", base.Add(-time.Hour)))
		mustAppend(t, s, finishedAt("run-old", base.Add(-time.Hour).Add(time.Minute)))
		mustAppend(t, s, startedAt("run-new", base))

		activity, err := s.AgentActivity(ctx, domain.RunFilter{})
		if err != nil {
			t.Fatalf("AgentActivity: %v", err)
		}
		// An agent whose last run is still going reads as running, even though
		// most of its history finished.
		if activity[0].LastPhase != "running" {
			t.Errorf("lastPhase = %q, want the newest run's phase", activity[0].LastPhase)
		}
	})

	run(t, "cost is summed per agent", func(t *testing.T, s Store) {
		ctx := context.Background()

		mustAppend(t, s, startedAt("run-a", base))
		priced := step("run-a", domain.StepPlanned)
		priced.At = base.Add(time.Second)
		priced.Cost = domain.Cost{Micros: 7_500}
		mustAppend(t, s, priced)

		activity, err := s.AgentActivity(ctx, domain.RunFilter{})
		if err != nil {
			t.Fatalf("AgentActivity: %v", err)
		}
		if activity[0].CostMicros != 7_500 {
			t.Errorf("cost = %d, want 7500 micros", activity[0].CostMicros)
		}
	})

	run(t, "a scope bound counts only that area's runs", func(t *testing.T, s Store) {
		ctx := context.Background()

		mustAppend(t, s, startedAt("run-cx", base))
		elsewhere := startedAt("run-mkt", base)
		elsewhere.Scope = domain.Scope{Company: "acme", Area: "marketing"}
		elsewhere.AgentID = "lead-qualifier"
		mustAppend(t, s, elsewhere)

		activity, err := s.AgentActivity(ctx, domain.RunFilter{Scope: domain.Scope{Company: "acme", Area: "cx"}})
		if err != nil {
			t.Fatalf("AgentActivity: %v", err)
		}
		if len(activity) != 1 || activity[0].AgentID != "triage" {
			t.Errorf("AgentActivity = %v, want only the agent running in cx", activity)
		}
	})
}

func TestScopesContract(t *testing.T) {
	base := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	seed := func(t *testing.T, s Store) {
		t.Helper()
		mustAppend(t, s, startedAt("run-cx", base))

		marketing := startedAt("run-mkt", base)
		marketing.Scope = domain.Scope{Company: "acme", Area: "marketing"}
		mustAppend(t, s, marketing)

		other := startedAt("run-other", base)
		other.Scope = domain.Scope{Company: "outra", Area: "cx"}
		mustAppend(t, s, other)
	}

	run(t, "a listing narrowed to one area shows only it", func(t *testing.T, s Store) {
		seed(t, s)

		page, err := s.ListRuns(context.Background(), domain.RunFilter{
			Scopes: []domain.Scope{{Company: "acme", Area: "cx"}},
		}, "", 50)
		if err != nil {
			t.Fatalf("ListRuns: %v", err)
		}
		if len(page) != 1 || page[0].RunID != "run-cx" {
			t.Errorf("ListRuns = %v, want only the run in cx", kindsOfPage(page))
		}
	})

	run(t, "several scopes are matched as any of them", func(t *testing.T, s Store) {
		seed(t, s)

		// Somebody holding a permission in two areas sees both, and nothing
		// else — which is why this is a filter and not a post-read discard.
		page, err := s.ListRuns(context.Background(), domain.RunFilter{
			Scopes: []domain.Scope{
				{Company: "acme", Area: "cx"},
				{Company: "acme", Area: "marketing"},
			},
		}, "", 50)
		if err != nil {
			t.Fatalf("ListRuns: %v", err)
		}
		if len(page) != 2 {
			t.Errorf("ListRuns = %v, want both granted areas", kindsOfPage(page))
		}
	})

	run(t, "a company-wide scope covers its areas and stops at the company", func(t *testing.T, s Store) {
		seed(t, s)

		page, err := s.ListRuns(context.Background(), domain.RunFilter{
			Scopes: []domain.Scope{{Company: "acme"}},
		}, "", 50)
		if err != nil {
			t.Fatalf("ListRuns: %v", err)
		}
		if len(page) != 2 {
			t.Errorf("ListRuns = %v, want both areas of acme and nothing from outra", kindsOfPage(page))
		}
	})

	run(t, "cost and stats narrow the same way", func(t *testing.T, s Store) {
		ctx := context.Background()
		seed(t, s)
		only := domain.RunFilter{
			Scopes: []domain.Scope{{Company: "acme", Area: "cx"}},
			Until:  base.Add(time.Hour),
		}

		stats, err := s.Stats(ctx, only)
		if err != nil {
			t.Fatalf("Stats: %v", err)
		}
		if stats.Total != 1 {
			t.Errorf("Stats.Total = %d, want only the granted area", stats.Total)
		}

		buckets, err := s.CostRollup(ctx, only, "area")
		if err != nil {
			t.Fatalf("CostRollup: %v", err)
		}
		if len(buckets) != 1 || buckets[0].Key != "cx" {
			t.Errorf("CostRollup = %v, want only cx", buckets)
		}
	})
}

func TestSpentSinceContract(t *testing.T) {
	base := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	run(t, "only the window counts, because a ceiling covers a period", func(t *testing.T, s Store) {
		ctx := context.Background()

		old := startedAt("run-old", base.Add(-48*time.Hour))
		mustAppend(t, s, old)
		priced := step("run-old", domain.StepPlanned)
		priced.At = base.Add(-48 * time.Hour)
		priced.Cost = domain.Cost{Micros: 90_000}
		mustAppend(t, s, priced)

		mustAppend(t, s, startedAt("run-now", base))
		recent := step("run-now", domain.StepPlanned)
		recent.At = base
		recent.Cost = domain.Cost{Micros: 10_000}
		mustAppend(t, s, recent)

		spent, err := s.SpentSince(ctx, domain.Scope{Company: "acme", Area: "cx"}, base.Add(-time.Hour))
		if err != nil {
			t.Fatalf("SpentSince: %v", err)
		}
		// A monthly ceiling that counted last month's spend would cut off the
		// first run of every month.
		if spent.Micros != 10_000 {
			t.Errorf("Micros = %d, want only what was spent inside the window", spent.Micros)
		}
	})

	run(t, "only the scope counts", func(t *testing.T, s Store) {
		ctx := context.Background()

		mustAppend(t, s, startedAt("run-cx", base))
		mine := step("run-cx", domain.StepPlanned)
		mine.At = base
		mine.Cost = domain.Cost{Micros: 5_000}
		mustAppend(t, s, mine)

		elsewhere := startedAt("run-mkt", base)
		elsewhere.Scope = domain.Scope{Company: "acme", Area: "marketing"}
		mustAppend(t, s, elsewhere)
		theirs := step("run-mkt", domain.StepPlanned)
		theirs.At = base
		theirs.Scope = elsewhere.Scope
		theirs.Cost = domain.Cost{Micros: 70_000}
		mustAppend(t, s, theirs)

		spent, err := s.SpentSince(ctx, domain.Scope{Company: "acme", Area: "cx"}, base.Add(-time.Hour))
		if err != nil {
			t.Fatalf("SpentSince: %v", err)
		}
		if spent.Micros != 5_000 {
			t.Errorf("Micros = %d, want only this area's spend", spent.Micros)
		}
	})

	run(t, "a company-wide question sums its areas", func(t *testing.T, s Store) {
		ctx := context.Background()

		for _, area := range []domain.AreaID{"cx", "marketing"} {
			id := domain.RunID("run-" + area)
			opened := startedAt(id, base)
			opened.Scope = domain.Scope{Company: "acme", Area: area}
			mustAppend(t, s, opened)

			priced := step(id, domain.StepPlanned)
			priced.At = base
			priced.Scope = opened.Scope
			priced.Cost = domain.Cost{Micros: 20_000}
			mustAppend(t, s, priced)
		}

		spent, err := s.SpentSince(ctx, domain.Scope{Company: "acme"}, base.Add(-time.Hour))
		if err != nil {
			t.Fatalf("SpentSince: %v", err)
		}
		if spent.Micros != 40_000 {
			t.Errorf("Micros = %d, want both areas of the company", spent.Micros)
		}
	})
}

// Throughput answers what the overview asks: how the day is going, hour by
// hour, split by what became of each run. Aggregated in the store for the same
// reason the tallies are — reading every run into the process to bucket it
// makes the console's cost grow with the installation's history.

func TestThroughputContract(t *testing.T) {
	base := time.Date(2026, 8, 11, 9, 30, 0, 0, time.UTC)

	run(t, "buckets runs by the hour they started", func(t *testing.T, s Store) {
		ctx := context.Background()

		mustAppend(t, s, startedAt("run-a", base))
		mustAppend(t, s, startedAt("run-b", base.Add(20*time.Minute)))
		mustAppend(t, s, startedAt("run-c", base.Add(90*time.Minute)))

		got, err := s.Throughput(ctx, domain.RunFilter{Since: base.Add(-time.Hour)})
		if err != nil {
			t.Fatalf("Throughput: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("buckets = %d, want 2 (got %+v)", len(got), got)
		}
		if !got[0].At.Equal(base.Truncate(time.Hour)) {
			t.Errorf("first bucket at %s, want the hour %s falls in", got[0].At, base)
		}
		if got[0].Total != 2 || got[1].Total != 1 {
			t.Errorf("totals = %d, %d; want 2, 1", got[0].Total, got[1].Total)
		}
	})

	run(t, "splits each hour by what became of the run", func(t *testing.T, s Store) {
		ctx := context.Background()

		mustAppend(t, s, startedAt("run-done", base))
		mustAppend(t, s, finishedAt("run-done", base.Add(time.Minute)))
		mustAppend(t, s, startedAt("run-open", base.Add(5*time.Minute)))

		got, err := s.Throughput(ctx, domain.RunFilter{Since: base.Add(-time.Hour)})
		if err != nil {
			t.Fatalf("Throughput: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("buckets = %d, want 1", len(got))
		}
		if got[0].ByPhase["finished"] != 1 || got[0].ByPhase["running"] != 1 {
			t.Errorf("byPhase = %v, want one finished and one running", got[0].ByPhase)
		}
	})

	run(t, "returns the hours in order, oldest first", func(t *testing.T, s Store) {
		ctx := context.Background()

		// Written newest first on purpose: a chart that trusted insertion
		// order would draw the day backwards.
		mustAppend(t, s, startedAt("run-late", base.Add(3*time.Hour)))
		mustAppend(t, s, startedAt("run-early", base))

		got, err := s.Throughput(ctx, domain.RunFilter{Since: base.Add(-time.Hour)})
		if err != nil {
			t.Fatalf("Throughput: %v", err)
		}
		if len(got) != 2 || !got[0].At.Before(got[1].At) {
			t.Fatalf("buckets = %+v, want oldest first", got)
		}
	})

	run(t, "honours the scope, like every other tally", func(t *testing.T, s Store) {
		ctx := context.Background()

		mine := startedAt("run-cx", base)
		theirs := startedAt("run-mkt", base)
		theirs.Scope = domain.Scope{Company: "acme", Area: "marketing"}
		mustAppend(t, s, mine)
		mustAppend(t, s, theirs)

		got, err := s.Throughput(ctx, domain.RunFilter{Scope: domain.Scope{Company: "acme", Area: "cx"}})
		if err != nil {
			t.Fatalf("Throughput: %v", err)
		}
		if len(got) != 1 || got[0].Total != 1 {
			t.Errorf("buckets = %+v, want only the run in cx", got)
		}
	})
}
