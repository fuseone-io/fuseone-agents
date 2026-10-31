package ledger_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/testdb"
)

/*
The work-item claim, against both stores (#206).

A claim is a work_item_claimed step the ledger judges while it seals it: one
card per run, and one open run per card per agent. "Open" is the run's phase,
read at that moment, so nothing ever writes a release — a run that ends, however
it ends, simply stops holding.
*/

const (
	card      = domain.WorkItemRef("github:fuseone-io/fuseone-agents#123")
	otherCard = domain.WorkItemRef("github:fuseone-io/fuseone-agents#124")
)

func openRunOf(t *testing.T, s Store, runID domain.RunID, agent domain.AgentID) {
	t.Helper()
	started := step(runID, domain.StepRunStarted)
	started.AgentID = agent
	mustAppend(t, s, started)
}

func claimStep(t *testing.T, runID domain.RunID, agent domain.AgentID, ref domain.WorkItemRef) domain.Step {
	t.Helper()
	claimed := step(runID, domain.StepWorkItemClaimed)
	claimed.AgentID = agent
	claimed.Payload = mustJSON(t, domain.WorkItemClaimedPayload{Ref: ref})
	return claimed
}

func claim(t *testing.T, s Store, runID domain.RunID, agent domain.AgentID, ref domain.WorkItemRef) error {
	t.Helper()
	_, err := s.Append(context.Background(), claimStep(t, runID, agent, ref))
	return err
}

func headSeq(t *testing.T, s Store, runID domain.RunID) int64 {
	t.Helper()
	head, err := s.Head(context.Background(), runID)
	if err != nil {
		t.Fatalf("Head(%s): %v", runID, err)
	}
	return head.Seq
}

func TestWorkItemClaim_sameRunSameRefTwice_isAnIdempotentSuccess(t *testing.T) {
	run(t, "a retry of the same claim succeeds", func(t *testing.T, s Store) {
		openRunOf(t, s, "run-a", "triage")
		if err := claim(t, s, "run-a", "triage", card); err != nil {
			t.Fatalf("first claim: %v", err)
		}
		if err := claim(t, s, "run-a", "triage", card); err != nil {
			t.Fatalf("second claim of the same card by the same run = %v, want success", err)
		}
	})
}

func TestWorkItemClaim_sameRunDifferentRef_refused(t *testing.T) {
	run(t, "a run holds at most one card", func(t *testing.T, s Store) {
		openRunOf(t, s, "run-a", "triage")
		if err := claim(t, s, "run-a", "triage", card); err != nil {
			t.Fatalf("first claim: %v", err)
		}
		before := headSeq(t, s, "run-a")

		err := claim(t, s, "run-a", "triage", "github:fuseone-io/fuseone-agents#124")
		if !errors.Is(err, domain.ErrWorkItemRunHoldsAnother) {
			t.Fatalf("claim of a second card = %v, want ErrWorkItemRunHoldsAnother", err)
		}
		if after := headSeq(t, s, "run-a"); after != before {
			t.Fatalf("a refused claim moved the head from %d to %d; refusals are not steps", before, after)
		}
	})
}

func TestWorkItemClaim_anotherOpenRunOfTheSameAgent_refused(t *testing.T) {
	run(t, "one open run per card per agent", func(t *testing.T, s Store) {
		openRunOf(t, s, "run-a", "triage")
		openRunOf(t, s, "run-b", "triage")
		if err := claim(t, s, "run-a", "triage", card); err != nil {
			t.Fatalf("first run's claim: %v", err)
		}
		if err := claim(t, s, "run-b", "triage", card); !errors.Is(err, domain.ErrWorkItemHeld) {
			t.Fatalf("second run's claim = %v, want ErrWorkItemHeld", err)
		}
	})
}

// A run waiting on a person is the case the claim exists for: the board has
// not changed, and the next firing picks the same card.
func TestWorkItemClaim_parkedOrAwaitingApproval_keepsTheClaim(t *testing.T) {
	for _, pause := range []domain.StepKind{domain.StepParked, domain.StepApprovalRequested, domain.StepToolCalled} {
		run(t, "a run held at "+string(pause)+" keeps its card", func(t *testing.T, s Store) {
			openRunOf(t, s, "run-a", "triage")
			openRunOf(t, s, "run-b", "triage")
			if err := claim(t, s, "run-a", "triage", card); err != nil {
				t.Fatalf("first run's claim: %v", err)
			}
			mustAppend(t, s, step("run-a", pause))

			if err := claim(t, s, "run-b", "triage", card); !errors.Is(err, domain.ErrWorkItemHeld) {
				t.Fatalf("claim while the holder is at %s = %v, want ErrWorkItemHeld", pause, err)
			}
		})
	}
}

// Whatever ended the run, the card is free again, and nothing wrote a release:
// the only steps after the claim are the run's own ending.
func TestWorkItemClaim_terminalHolder_releasesWithNoReleaseWrite(t *testing.T) {
	endings := map[string][]domain.Step{
		"finished": {step("run-a", domain.StepRunFinished)},
		"failed":   {step("run-a", domain.StepFailed)},
		"abandoned": {withPayload(t, step("run-a", domain.StepAbandoned),
			domain.AbandonedPayload{By: "ana", Reason: "stuck"})},
		// Killed mid-turn: the call was recorded, the worker died before the
		// result, and the run was then ended by a person.
		"killed mid-turn": {
			withPayload(t, step("run-a", domain.StepToolCalled),
				domain.ToolCalledPayload{Tool: "github.add_comment", Effect: domain.EffectWrite}),
			withPayload(t, step("run-a", domain.StepAbandoned),
				domain.AbandonedPayload{By: "ana", Reason: "worker lost"}),
		},
	}
	for name, ending := range endings {
		run(t, "a "+name+" run no longer holds its card", func(t *testing.T, s Store) {
			openRunOf(t, s, "run-a", "triage")
			openRunOf(t, s, "run-b", "triage")
			if err := claim(t, s, "run-a", "triage", card); err != nil {
				t.Fatalf("first run's claim: %v", err)
			}
			for _, end := range ending {
				mustAppend(t, s, end)
			}

			if err := claim(t, s, "run-b", "triage", card); err != nil {
				t.Fatalf("claim after the holder ended (%s) = %v, want success", name, err)
			}
		})
	}
}

// Compensating is not over: the run still has something to take back.
func TestWorkItemClaim_abandonedWithCompensation_stillHolds(t *testing.T) {
	run(t, "a compensating run keeps its card", func(t *testing.T, s Store) {
		openRunOf(t, s, "run-a", "triage")
		openRunOf(t, s, "run-b", "triage")
		if err := claim(t, s, "run-a", "triage", card); err != nil {
			t.Fatalf("first run's claim: %v", err)
		}
		mustAppend(t, s, withPayload(t, step("run-a", domain.StepAbandoned),
			domain.AbandonedPayload{By: "ana", Reason: "undo", Compensate: true}))

		if err := claim(t, s, "run-b", "triage", card); !errors.Is(err, domain.ErrWorkItemHeld) {
			t.Fatalf("claim while the holder compensates = %v, want ErrWorkItemHeld", err)
		}
	})
}

func TestWorkItemClaim_differentAgents_mayHoldTheSameCard(t *testing.T) {
	run(t, "the uniqueness is per agent", func(t *testing.T, s Store) {
		openRunOf(t, s, "run-a", "triage")
		openRunOf(t, s, "run-b", "developer")
		if err := claim(t, s, "run-a", "triage", card); err != nil {
			t.Fatalf("triage's claim: %v", err)
		}
		if err := claim(t, s, "run-b", "developer", card); err != nil {
			t.Fatalf("developer's claim of the same card = %v, want success", err)
		}
	})
}

func TestWorkItemClaim_withNoAgentOrAMalformedRef_refused(t *testing.T) {
	run(t, "a claim the ledger cannot key is refused", func(t *testing.T, s Store) {
		openRunOf(t, s, "run-a", "triage")
		for name, st := range map[string]domain.Step{
			"no agent":      claimStep(t, "run-a", "", card),
			"empty ref":     claimStep(t, "run-a", "triage", ""),
			"untrimmed ref": claimStep(t, "run-a", "triage", " card-7"),
		} {
			if _, err := s.Append(context.Background(), st); !errors.Is(err, domain.ErrWorkItemInvalid) {
				t.Errorf("%s: Append = %v, want ErrWorkItemInvalid", name, err)
			}
		}
	})
}

// Cost per card counts every run that claimed it, including one that failed
// before its first write — the run nothing else would ever attribute.
func TestWorkItemCost_includesARunThatFailedBeforeAnyWrite(t *testing.T) {
	run(t, "cost by work item", func(t *testing.T, s Store) {
		openRunOf(t, s, "run-a", "triage")
		if err := claim(t, s, "run-a", "triage", card); err != nil {
			t.Fatalf("claim: %v", err)
		}
		planned := step("run-a", domain.StepPlanned)
		planned.Cost = domain.Cost{Micros: 700, InputTokens: 10}
		mustAppend(t, s, planned)
		mustAppend(t, s, step("run-a", domain.StepFailed))

		openRunOf(t, s, "run-b", "developer")
		if err := claim(t, s, "run-b", "developer", card); err != nil {
			t.Fatalf("claim: %v", err)
		}
		other := step("run-b", domain.StepPlanned)
		other.Cost = domain.Cost{Micros: 300, InputTokens: 10}
		mustAppend(t, s, other)

		openRunOf(t, s, "run-c", "triage")
		if err := claim(t, s, "run-c", "triage", "card-elsewhere"); err != nil {
			t.Fatalf("claim: %v", err)
		}
		unrelated := step("run-c", domain.StepPlanned)
		unrelated.Cost = domain.Cost{Micros: 5_000, InputTokens: 10}
		mustAppend(t, s, unrelated)

		got, err := s.WorkItemCost(context.Background(), domain.RunFilter{}, card)
		if err != nil {
			t.Fatalf("WorkItemCost: %v", err)
		}
		want := domain.WorkItemCost{Ref: card, Runs: 2, Micros: 1_000}
		if got != want {
			t.Fatalf("WorkItemCost = %+v, want %+v", got, want)
		}
	})
}

func withPayload(t *testing.T, s domain.Step, payload any) domain.Step {
	t.Helper()
	s.Payload = mustJSON(t, payload)
	return s
}

// --- Postgres only ----------------------------------------------------------

func openWorkItemPostgres(t *testing.T) (*ledger.Postgres, *pgxpool.Pool) {
	t.Helper()
	dsn := testdb.DSN(t)
	if dsn == "" {
		requireDatabase(t, dsn)
		t.Skip("TEST_DATABASE_URL is unset")
	}
	store, ok := openPostgres(t, dsn).(*ledger.Postgres)
	if !ok {
		t.Fatal("openPostgres did not return a Postgres store")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return store, pool
}

/*
Two runs of one agent claim one card at once: exactly one wins.

Staged rather than hoped for. The winner's claim is sealed inside a transaction
held open; the loser's Append is let go and observed waiting on the work-item
lock in pg_stat_activity; only then does the winner commit. A loser that judged
before taking the lock — or in the same statement as the lock, on a snapshot
older than the wait — would see no holder and take the card too.
*/
func TestWorkItemClaim_concurrentClaimsOfOneCard_exactlyOneSucceeds(t *testing.T) {
	store, pool := openWorkItemPostgres(t)
	ctx := t.Context()
	openRunOf(t, store, "run-winner", "triage")
	openRunOf(t, store, "run-loser", "triage")

	winner, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin the winner: %v", err)
	}
	defer func() { _ = winner.Rollback(context.Background()) }()
	if _, err := ledger.AppendInTx(ctx, winner, claimStep(t, "run-winner", "triage", card)); err != nil {
		t.Fatalf("hold the winner's claim: %v", err)
	}

	lost := make(chan error, 1)
	go func() {
		_, err := store.Append(ctx, claimStep(t, "run-loser", "triage", card))
		lost <- err
	}()

	waitForWorkItemLockWait(t, pool)
	if err := winner.Commit(ctx); err != nil {
		t.Fatalf("commit the winner: %v", err)
	}

	if err := <-lost; !errors.Is(err, domain.ErrWorkItemHeld) {
		t.Fatalf("the loser's claim = %v, want ErrWorkItemHeld", err)
	}
	var holders int
	if err := pool.QueryRow(ctx,
		`select count(*) from run_work_items where ref = $1`, string(card),
	).Scan(&holders); err != nil {
		t.Fatalf("count holders: %v", err)
	}
	if holders != 1 {
		t.Fatalf("holders of the card = %d, want exactly one", holders)
	}
}

// waitForWorkItemLockWait returns once some backend is blocked on the
// work-item advisory lock — the loser parked behind the winner's transaction.
func waitForWorkItemLockWait(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := pool.QueryRow(t.Context(), `
			select count(*) from pg_stat_activity
			where wait_event_type = 'Lock' and wait_event = 'advisory'
			  and query like '%work_item:%'
			  and pid <> pg_backend_pid()`).Scan(&waiting); err != nil {
			t.Fatalf("read lock waits: %v", err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the loser never waited on the work-item lock; the race was not staged")
}

// The release is derived, never written: the holder's row is still there after
// it ended, unchanged, and the table refuses to have it changed or removed.
func TestWorkItemClaim_releaseLeavesTheClaimRowAndTheRowCannotBeChanged(t *testing.T) {
	store, pool := openWorkItemPostgres(t)
	ctx := t.Context()
	openRunOf(t, store, "run-a", "triage")
	if err := claim(t, store, "run-a", "triage", card); err != nil {
		t.Fatalf("claim: %v", err)
	}
	mustAppend(t, store, step("run-a", domain.StepRunFinished))

	var ref string
	if err := pool.QueryRow(ctx,
		`select ref from run_work_items where run_id = 'run-a'`).Scan(&ref); err != nil {
		t.Fatalf("read the ended run's claim row: %v", err)
	}
	if ref != string(card) {
		t.Fatalf("claim row ref = %q, want %q", ref, card)
	}
	if _, err := pool.Exec(ctx, `update run_work_items set ref = 'x' where run_id = 'run-a'`); err == nil {
		t.Fatal("run_work_items accepted an update; a claim is never amended")
	}
	if _, err := pool.Exec(ctx, `delete from run_work_items where run_id = 'run-a'`); err == nil {
		t.Fatal("run_work_items accepted a delete; a claim is released by the run's phase, not by a write")
	}
}

// --- simulations ------------------------------------------------------------

func openSimulatedRun(t *testing.T, s Store, runID domain.RunID, agent domain.AgentID) {
	t.Helper()
	started := step(runID, domain.StepRunStarted)
	started.AgentID = agent
	started.Payload = mustJSON(t, domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: "sim-1",
	})
	mustAppend(t, s, started)
}

/*
A simulation rehearses the claim; it never takes one.

Left durable, a simulated run waiting on an approval would hold the card
against the real run of the same agent indefinitely, and its spend would be
counted against the card. Inside its own run the claim still behaves — success,
an idempotent retry, a second card refused — so the rehearsal stays faithful.
*/
func TestWorkItemClaim_simulatedRun_holdsNothingAndCountsNothing(t *testing.T) {
	run(t, "a simulated claim is inert outside its own run", func(t *testing.T, s Store) {
		openSimulatedRun(t, s, "run-sim", "triage")
		if err := claim(t, s, "run-sim", "triage", card); err != nil {
			t.Fatalf("simulated claim = %v, want the rehearsal to succeed", err)
		}
		if err := claim(t, s, "run-sim", "triage", card); err != nil {
			t.Fatalf("simulated retry = %v, want success", err)
		}
		if err := claim(t, s, "run-sim", "triage", otherCard); !errors.Is(err, domain.ErrWorkItemRunHoldsAnother) {
			t.Fatalf("simulated second card = %v, want ErrWorkItemRunHoldsAnother", err)
		}
		spent := step("run-sim", domain.StepPlanned)
		spent.Cost = domain.Cost{Micros: 900, InputTokens: 10}
		mustAppend(t, s, spent)
		mustAppend(t, s, step("run-sim", domain.StepApprovalRequested))

		openRunOf(t, s, "run-real", "triage")
		if err := claim(t, s, "run-real", "triage", card); err != nil {
			t.Fatalf("real claim beside an open simulation = %v, want success", err)
		}
		got, err := s.WorkItemCost(context.Background(), domain.RunFilter{}, card)
		if err != nil {
			t.Fatalf("WorkItemCost: %v", err)
		}
		if got.Runs != 1 || got.Micros != 0 {
			t.Fatalf("WorkItemCost = %+v, want only the real run", got)
		}
	})
}

func TestWorkItemClaim_simulatedRun_leavesNoProjectionRow(t *testing.T) {
	store, pool := openWorkItemPostgres(t)
	openSimulatedRun(t, store, "run-sim", "triage")
	if err := claim(t, store, "run-sim", "triage", card); err != nil {
		t.Fatalf("simulated claim: %v", err)
	}
	var rows int
	if err := pool.QueryRow(t.Context(),
		`select count(*) from run_work_items where run_id = 'run-sim'`).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 0 {
		t.Fatalf("run_work_items rows for a simulated run = %d, want none", rows)
	}
}

// Cost per card is a financial aggregate, narrowed like every other: a caller
// limited to one area sees the runs of that area, never another area's or
// another company's runs that happen to name the same card.
func TestWorkItemCost_narrowsToTheCallersScopes(t *testing.T) {
	run(t, "cost by work item respects scope", func(t *testing.T, s Store) {
		// An agent lives in one area, so each scope has its own.
		for _, one := range []struct {
			run    domain.RunID
			agent  domain.AgentID
			scope  domain.Scope
			micros int64
		}{
			{"run-cx", "triage-cx", domain.Scope{Company: "acme", Area: "cx"}, 100},
			{"run-ops", "triage-ops", domain.Scope{Company: "acme", Area: "ops"}, 20},
			{"run-other", "triage-globex", domain.Scope{Company: "globex", Area: "cx"}, 3},
		} {
			started := step(one.run, domain.StepRunStarted)
			started.Scope, started.AgentID = one.scope, one.agent
			mustAppend(t, s, started)
			claimed := claimStep(t, one.run, one.agent, card)
			claimed.Scope = one.scope
			mustAppend(t, s, claimed)
			planned := step(one.run, domain.StepPlanned)
			planned.Scope, planned.AgentID = one.scope, one.agent
			planned.Cost = domain.Cost{Micros: one.micros, InputTokens: 1}
			mustAppend(t, s, planned)
		}

		cases := map[string]struct {
			filter domain.RunFilter
			want   domain.WorkItemCost
		}{
			"one area": {domain.RunFilter{Scopes: []domain.Scope{{Company: "acme", Area: "cx"}}},
				domain.WorkItemCost{Ref: card, Runs: 1, Micros: 100}},
			"a whole company": {domain.RunFilter{Scopes: []domain.Scope{{Company: "acme"}}},
				domain.WorkItemCost{Ref: card, Runs: 2, Micros: 120}},
			"the installation": {domain.RunFilter{Scopes: []domain.Scope{{Company: domain.Installation}}},
				domain.WorkItemCost{Ref: card, Runs: 3, Micros: 123}},
		}
		for name, tc := range cases {
			got, err := s.WorkItemCost(context.Background(), tc.filter, card)
			if err != nil {
				t.Fatalf("%s: WorkItemCost: %v", name, err)
			}
			if got != tc.want {
				t.Errorf("%s: WorkItemCost = %+v, want %+v", name, got, tc.want)
			}
		}
	})
}
