package admin_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

/*
A correction keeps its own copy of what a run was about, so the corpus survives
the run. That copy is filed under the agent rather than under the run, so
erasing the run did not reach it: the run read as erased while the copy still
returned the bytes, and the operator had no signal the copy existed.
*/

type correction struct {
	agent, id, run, ref, note string
}

func recordCorrection(t *testing.T, pool *pgxpool.Pool, c correction) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		insert into regression_cases (agent_id, case_id, input_ref, from_run, note)
		values ($1, $2, $3, $4, $5)`, c.agent, c.id, c.ref, c.run, c.note); err != nil {
		t.Fatalf("record correction: %v", err)
	}
}

// replay opens the simulated run a battery opens for a case.
func replay(t *testing.T, pool *pgxpool.Pool, agent string, run domain.RunID, kase string) {
	t.Helper()
	payload, err := json.Marshal(domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: "battery-" + kase, Case: kase,
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	_, err = ledger.NewPostgres(pool).Append(context.Background(), domain.Step{
		RunID: run, Seq: 1, Kind: domain.StepRunStarted, AgentID: domain.AgentID(agent),
		Scope:   domain.Scope{Company: "acme", Area: "suporte"},
		IdemKey: string(run), VersionID: "v1", Payload: payload,
		At: time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
}

func cleanCorpus(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `delete from regression_cases`); err != nil {
		t.Fatalf("clean: %v", err)
	}
}

func noteOf(t *testing.T, pool *pgxpool.Pool, id string) string {
	t.Helper()
	var note string
	if err := pool.QueryRow(context.Background(),
		`select note from regression_cases where case_id = $1`, id).Scan(&note); err != nil {
		t.Fatalf("read note: %v", err)
	}
	return note
}

func TestForSubject_reachesTheCorpusCopyOfANamedRun(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()

	if _, err := content.Put(ctx, "run-asked", 1, []byte(`{"email":"ana@example.com"}`)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	copied, err := content.PutFor(ctx, "regression", "suporte", 1, []byte(`{"email":"ana@example.com"}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	recordCorrection(t, pool, correction{"suporte", "reg-1", "run-asked", copied, "ana asked twice"})

	if _, err := erasures.ForSubject(ctx, "dpo", domain.Scope{}, []domain.RunID{"run-asked"}, "request"); err != nil {
		t.Fatalf("ForSubject: %v", err)
	}

	if _, err := content.Get(ctx, copied); !errors.Is(err, ledger.ErrErased) {
		t.Fatalf("corpus copy after erasure: err = %v, want erased", err)
	}
	if got := noteOf(t, pool, "reg-1"); got != "" {
		t.Fatalf("note after erasure = %q, want it gone", got)
	}
}

func TestForSubject_leavesACorrectionMadeFromAnotherRunAlone(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()

	kept, err := content.PutFor(ctx, "regression", "suporte", 1, []byte(`{"email":"bia@example.com"}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	gone, err := content.PutFor(ctx, "regression", "suporte", 1, []byte(`{"email":"ana@example.com"}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	recordCorrection(t, pool, correction{"suporte", "reg-kept", "run-other", kept, "about bia"})
	recordCorrection(t, pool, correction{"suporte", "reg-gone", "run-asked", gone, "about ana"})

	if _, err := erasures.ForSubject(ctx, "dpo", domain.Scope{}, []domain.RunID{"run-asked"}, "request"); err != nil {
		t.Fatalf("ForSubject: %v", err)
	}

	if _, err := content.Get(ctx, kept); err != nil {
		t.Fatalf("another correction's copy: %v", err)
	}
	if got := noteOf(t, pool, "reg-kept"); got != "about bia" {
		t.Fatalf("another correction's note = %q", got)
	}
}

// A battery opens a new run holding the same input every time it runs, so an
// erasure that stopped at the corpus would leave one copy per night.
func TestForSubject_reachesTheRunsThatReplayedTheCorrection(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()
	// The ledger has no delete, so a second run of this test needs new runs.
	stamp := fmt.Sprint(time.Now().UnixNano())
	replayRun := domain.RunID("run-replay-" + stamp)
	otherRun := domain.RunID("run-replay-other-" + stamp)

	copied, err := content.PutFor(ctx, "regression", "suporte", 1, []byte(`{"email":"ana@example.com"}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	recordCorrection(t, pool, correction{"suporte", "reg-1", "run-asked", copied, ""})
	replay(t, pool, "suporte", replayRun, "reg-1")
	replay(t, pool, "suporte", otherRun, "reg-2")
	replayed, err := content.Put(ctx, replayRun, 1, []byte(`{"email":"ana@example.com"}`))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	unrelated, err := content.Put(ctx, otherRun, 1, []byte(`{"email":"bia@example.com"}`))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if _, err := erasures.ForSubject(ctx, "dpo", domain.Scope{}, []domain.RunID{"run-asked"}, "request"); err != nil {
		t.Fatalf("ForSubject: %v", err)
	}

	if _, err := content.Get(ctx, replayed); !errors.Is(err, ledger.ErrErased) {
		t.Fatalf("replay's copy: err = %v, want erased", err)
	}
	if _, err := content.Get(ctx, unrelated); err != nil {
		t.Fatalf("another case's replay: %v", err)
	}
}

// An erasure nobody can account for is indistinguishable from data loss, and
// a DPO asked to prove a request was honoured has to see this part of it too.
func TestForSubject_recordsHowManyCorrectionsItReached(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()

	copied, err := content.PutFor(ctx, "regression", "suporte", 1, []byte(`{"email":"ana@example.com"}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	recordCorrection(t, pool, correction{"suporte", "reg-1", "run-asked", copied, ""})

	if _, err := erasures.ForSubject(ctx, "dpo", domain.Scope{}, []domain.RunID{"run-asked"}, "request"); err != nil {
		t.Fatalf("ForSubject: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `
		select (detail->>'corpusCases')::int from admin_events
		where action = 'content.erased' order by event_id desc limit 1`).Scan(&count); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if count != 1 {
		t.Fatalf("corpusCases = %d, want 1", count)
	}
}

// A note is free text about the run, and it should not outlive the copy it
// was written about. Retention already takes the copy.
func TestSweep_clearsTheNoteOfACorrectionWhoseCopyAgedOut(t *testing.T) {
	future := time.Now().Add(6 * 365 * 24 * time.Hour)
	erasures, content := erasuresFor(t, func() time.Time { return future })
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()

	copied, err := content.PutFor(ctx, "regression", "suporte", 1, []byte(`{"email":"ana@example.com"}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	recordCorrection(t, pool, correction{"suporte", "reg-1", "run-asked", copied, "ana asked twice"})

	if _, err := erasures.Sweep(ctx); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if got := noteOf(t, pool, "reg-1"); got != "" {
		t.Fatalf("note after retention = %q, want it gone", got)
	}
}
