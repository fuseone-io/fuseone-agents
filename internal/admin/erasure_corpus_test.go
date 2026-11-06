package admin_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/testdb"
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
	open(t, pool, agent, run, domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: "battery-" + kase, Case: kase,
	})
}

func open(
	t *testing.T, pool *pgxpool.Pool, agent string, run domain.RunID, started domain.RunStartedPayload,
) {
	t.Helper()
	payload, err := json.Marshal(started)
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

// fresh is a run id no earlier pass of this test used: the ledger has no delete.
func fresh(prefix string) domain.RunID {
	return domain.RunID(prefix + fmt.Sprint(time.Now().UnixNano()))
}

// Identical input shares a reference. The replay of the other correction holds
// the same bytes, and leaving it readable would keep what was asked to go.
func TestForSubject_reachesTheReplayOfACorrectionSharingTheCopy(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()
	input := []byte(`{"email":"ana@example.com"}`)

	copied, err := content.PutFor(ctx, "regression", "suporte", 1, input)
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	recordCorrection(t, pool, correction{"suporte", "reg-a", "run-asked", copied, "first"})
	recordCorrection(t, pool, correction{"suporte", "reg-b", "run-other", copied, "second"})
	other := fresh("run-replay-b-")
	replay(t, pool, "suporte", other, "reg-b")
	replayed, err := content.Put(ctx, other, 1, input)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if _, err := erasures.ForSubject(ctx, "dpo", domain.Scope{}, []domain.RunID{"run-asked"}, "request"); err != nil {
		t.Fatalf("ForSubject: %v", err)
	}

	if _, err := content.Get(ctx, replayed); !errors.Is(err, ledger.ErrErased) {
		t.Fatalf("replay of the correction sharing the copy: err = %v, want erased", err)
	}
	if got := noteOf(t, pool, "reg-b"); got != "" {
		t.Fatalf("note of the correction sharing the copy = %q", got)
	}
}

// The case is gone, so nothing links the run to its replays except what the run
// itself recorded: which input it was opened with.
func TestForSubject_reachesReplaysAfterTheCorrectionWasDeleted(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()
	input := []byte(`{"email":"ana@example.com"}`)

	asked := fresh("run-asked-")
	inputRef, err := content.Put(ctx, asked, 1, input)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	open(t, pool, "suporte", asked, domain.RunStartedPayload{Trigger: "manual", InputRef: inputRef})
	replayRun := fresh("run-replay-")
	replay(t, pool, "suporte", replayRun, "reg-deleted")
	replayed, err := content.Put(ctx, replayRun, 1, input)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if _, err := erasures.ForSubject(ctx, "dpo", domain.Scope{}, []domain.RunID{asked}, "request"); err != nil {
		t.Fatalf("ForSubject: %v", err)
	}

	if _, err := content.Get(ctx, replayed); !errors.Is(err, ledger.ErrErased) {
		t.Fatalf("replay of a deleted correction: err = %v, want erased", err)
	}
}

// Erasure follows identical bytes only inside the agent, and only into its
// rehearsals. A real run is the operator's to name.
func TestForSubject_leavesWhatIsNotAReplayOfThisAgent(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()
	input := []byte(`{"email":"ana@example.com"}`)

	asked := fresh("run-asked-")
	inputRef, err := content.Put(ctx, asked, 1, input)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	open(t, pool, "suporte", asked, domain.RunStartedPayload{Trigger: "manual", InputRef: inputRef})

	realRun := fresh("run-real-")
	open(t, pool, "suporte", realRun, domain.RunStartedPayload{Trigger: "manual"})
	realContent, _ := content.Put(ctx, realRun, 1, input)
	otherAgent := fresh("run-other-agent-")
	replay(t, pool, "vendas", otherAgent, "reg-vendas")
	otherContent, _ := content.Put(ctx, otherAgent, 1, input)
	otherCopy, _ := content.PutFor(ctx, "regression", "vendas", 1, input)
	different := fresh("run-different-")
	replay(t, pool, "suporte", different, "reg-other")
	differentContent, _ := content.Put(ctx, different, 1, []byte(`{"email":"bia@example.com"}`))

	if _, err := erasures.ForSubject(ctx, "dpo", domain.Scope{}, []domain.RunID{asked}, "request"); err != nil {
		t.Fatalf("ForSubject: %v", err)
	}
	if _, err := content.Get(ctx, inputRef); !errors.Is(err, ledger.ErrErased) {
		t.Fatalf("the named run itself: err = %v, want erased", err)
	}

	for name, ref := range map[string]string{
		"a real run with the same bytes": realContent, "another agent's replay": otherContent,
		"another agent's copy": otherCopy, "a replay of other bytes": differentContent,
	} {
		if _, err := content.Get(ctx, ref); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// A correction being recorded read the run before the erasure and writes after
// it. Held, the erasure waits; once it runs, it finds what was written.
func TestForSubject_waitsForACorrectionBeingRecorded(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()
	store := regression.NewStore(pool)
	input := []byte(`{"email":"ana@example.com"}`)

	asked := fresh("run-asked-")
	inputRef, err := content.Put(ctx, asked, 1, input)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	open(t, pool, "suporte", asked, domain.RunStartedPayload{Trigger: "manual", InputRef: inputRef})

	entered, release, erased := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var copied string
	held := make(chan error, 1)
	go func() {
		held <- store.Hold(ctx, "suporte", func(ctx context.Context) error {
			close(entered)
			<-release
			var err error
			copied, err = content.RestoreFor(ctx, "regression", "suporte", 1, input)
			if err != nil {
				return err
			}
			_, err = pool.Exec(ctx, `insert into regression_cases
				(agent_id, case_id, input_ref, from_run) values ('suporte', 'reg-race', $1, $2)`,
				copied, string(asked))
			return err
		})
	}()
	<-entered
	go func() {
		_, err := erasures.ForSubject(ctx, "dpo", domain.Scope{}, []domain.RunID{asked}, "request")
		erased <- err
	}()

	select {
	case err := <-erased:
		t.Fatalf("the erasure did not wait for the correction: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-held; err != nil {
		t.Fatalf("Hold: %v", err)
	}
	if err := <-erased; err != nil {
		t.Fatalf("ForSubject: %v", err)
	}

	if _, err := content.Get(ctx, copied); !errors.Is(err, ledger.ErrErased) {
		t.Fatalf("copy written during the erasure: err = %v, want erased", err)
	}
}

// A request has to finish in time on an installation with years of history, so
// the search must not read in proportion to it.
func TestReplaySearch_usesTheDigestIndex(t *testing.T) {
	erasuresFor(t, time.Now)
	pool := openPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		insert into run_content (ref, owner_kind, run_id, seq, digest, bytes, full_bytes, truncated)
		select 'run://plan/' || g, 'run', 'plan-' || g, 1, md5(g::text), 'x', 1, false
		from generate_series(1, 5000) g;
		analyze run_content;`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	for name, query := range admin.CorpusSearchQueries() {
		rows, err := pool.Query(ctx, "explain "+query,
			[]string{"abc"}, []string{"suporte"})
		if err != nil {
			t.Fatalf("%s: explain: %v", name, err)
		}
		plan := ""
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatalf("%s: scan: %v", name, err)
			}
			plan += line + "\n"
		}
		rows.Close()
		if strings.Contains(plan, "Seq Scan on run_content") {
			t.Errorf("%s: plan reads all the content:\n%s", name, plan)
		}
		// Copies may use the owner index instead, which is bounded by the agent's
		// corpus. Replays have no such bound: an agent's content is its whole history.
		if name == "replays" && !strings.Contains(plan, "run_content_digest_idx") {
			t.Errorf("%s: plan does not use the digest index:\n%s", name, plan)
		}
		if strings.Contains(plan, "run_steps") {
			t.Errorf("%s: plan reads the ledger:\n%s", name, plan)
		}
	}
}

// Retention is the other way a copy goes. A battery that has read a case and has
// yet to open its runs would otherwise put expired bytes back into a new run.
func TestSweep_waitsForACopyInProgress(t *testing.T) {
	future := time.Now().Add(6 * 365 * 24 * time.Hour)
	erasures, content := erasuresFor(t, func() time.Time { return future })
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()
	store := regression.NewStore(pool)

	copied, err := content.PutFor(ctx, "regression", "suporte", 1, []byte(`{"email":"ana@example.com"}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}

	entered, release, swept := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	held := make(chan error, 1)
	go func() {
		held <- store.Hold(ctx, "suporte", func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	go func() {
		_, err := erasures.Sweep(ctx)
		swept <- err
	}()

	select {
	case err := <-swept:
		t.Fatalf("the sweep did not wait for the battery: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-held; err != nil {
		t.Fatalf("Hold: %v", err)
	}
	if err := <-swept; err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if _, err := content.Get(ctx, copied); !errors.Is(err, ledger.ErrErased) {
		t.Fatalf("copy after the sweep: err = %v, want erased", err)
	}
}

// Waiting at the fence must not hold what the work inside it needs.
func TestForSubject_waitingAtTheFence_leavesThePoolFree(t *testing.T) {
	config, err := pgxpool.ParseConfig(testdb.DSN(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	config.MaxConns = 1
	small, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer small.Close()
	erasuresFor(t, time.Now)
	pool := openPool(t)
	cleanCorpus(t, pool)
	store := regression.NewStore(small)
	erasures := admin.NewErasures(small, ledger.NewContent(small), admin.NewRetention(small, nil), store)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	copied, err := ledger.NewContent(pool).PutFor(ctx, "regression", "suporte", 1, []byte("x"))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	recordCorrection(t, pool, correction{"suporte", "reg-1", "run-asked", copied, ""})

	entered, release, erased := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	held := make(chan error, 1)
	go func() {
		held <- store.Hold(ctx, "suporte", func(ctx context.Context) error {
			close(entered)
			<-release
			_, err := store.List(ctx, "suporte")
			return err
		})
	}()
	<-entered
	go func() {
		_, err := erasures.ForSubject(ctx, "dpo", domain.Scope{}, []domain.RunID{"run-asked"}, "request")
		erased <- err
	}()
	time.Sleep(200 * time.Millisecond)
	close(release)

	if err := <-held; err != nil {
		t.Fatalf("Hold, with the erasure waiting: %v", err)
	}
	if err := <-erased; err != nil {
		t.Fatalf("ForSubject: %v", err)
	}
}

// A nightly battery is the platform duplicating bytes, not somebody collecting them
// again. Given a window of its own, each replay would keep them for nearly two.
func TestSweep_takesTheRecentReplaysOfACopyItTook(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()
	input := []byte(`{"email":"ana@example.com"}`)

	copied, err := content.PutFor(ctx, "regression", "suporte", 1, input)
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	if _, err := pool.Exec(ctx, `update run_content set created_at = now() - interval '6 years'
		where ref = $1`, copied); err != nil {
		t.Fatalf("age the copy: %v", err)
	}

	replayRun := fresh("run-replay-")
	replay(t, pool, "suporte", replayRun, "reg-1")
	replayed, err := content.Put(ctx, replayRun, 1, input)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	realRun := fresh("run-real-")
	open(t, pool, "suporte", realRun, domain.RunStartedPayload{Trigger: "manual"})
	realContent, _ := content.Put(ctx, realRun, 1, input)
	otherAgent := fresh("run-other-agent-")
	replay(t, pool, "vendas", otherAgent, "reg-vendas")
	otherContent, _ := content.Put(ctx, otherAgent, 1, input)
	otherBytes := fresh("run-other-bytes-")
	replay(t, pool, "suporte", otherBytes, "reg-2")
	otherBytesContent, _ := content.Put(ctx, otherBytes, 1, []byte(`{"email":"bia@example.com"}`))

	var digest string
	if err := pool.QueryRow(ctx, `select digest from run_content where ref = $1`, replayed).Scan(&digest); err != nil {
		t.Fatalf("read digest: %v", err)
	}
	var stepsBefore int
	if err := pool.QueryRow(ctx, `select count(*) from run_steps where run_id = $1`,
		string(replayRun)).Scan(&stepsBefore); err != nil || stepsBefore == 0 {
		t.Fatalf("steps = %d, %v", stepsBefore, err)
	}

	if _, err := erasures.Sweep(ctx); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	for name, ref := range map[string]string{"the copy": copied, "its recent replay": replayed} {
		if _, err := content.Get(ctx, ref); !errors.Is(err, ledger.ErrErased) {
			t.Errorf("%s: err = %v, want erased", name, err)
		}
	}
	for name, ref := range map[string]string{
		"a real run": realContent, "another agent's replay": otherContent,
		"a replay of other bytes": otherBytesContent,
	} {
		if _, err := content.Get(ctx, ref); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	// Only content: what happened stays on the record.
	var stepsAfter int
	if err := pool.QueryRow(ctx, `select count(*) from run_steps where run_id = $1`,
		string(replayRun)).Scan(&stepsAfter); err != nil || stepsAfter != stepsBefore {
		t.Errorf("steps after the sweep = %d, %v; want %d", stepsAfter, err, stepsBefore)
	}
	var kept string
	if err := pool.QueryRow(ctx, `select digest from run_content where ref = $1`, replayed).Scan(&kept); err != nil || kept != digest {
		t.Errorf("digest after the sweep = %q, %v; want %q", kept, err, digest)
	}
	var reason string
	if err := pool.QueryRow(ctx, `select erased_reason from run_content where ref = $1`, replayed).Scan(&reason); err != nil || reason != "retention" {
		t.Errorf("reason = %q, %v", reason, err)
	}
}

func TestSweep_recordsTheReplaysItTook(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()
	input := []byte(`{"email":"ana@example.com"}`)

	copied, err := content.PutFor(ctx, "regression", "suporte", 1, input)
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	if _, err := pool.Exec(ctx, `update run_content set created_at = now() - interval '6 years'
		where ref = $1`, copied); err != nil {
		t.Fatalf("age the copy: %v", err)
	}
	replayRun := fresh("run-replay-")
	replay(t, pool, "suporte", replayRun, "reg-1")
	if _, err := content.Put(ctx, replayRun, 1, input); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if _, err := erasures.Sweep(ctx); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	var replays int
	if err := pool.QueryRow(ctx, `
		select (detail->>'corpusReplays')::int from admin_events
		where action = 'content.expired' order by event_id desc limit 1`).Scan(&replays); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if replays != 1 {
		t.Fatalf("corpusReplays = %d, want 1", replays)
	}
}

// Retention runs every day, so what it follows must not read the whole content store.
func TestExpiredReplaySearch_startsFromTheCopies(t *testing.T) {
	erasuresFor(t, time.Now)
	pool := openPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		insert into run_content (ref, owner_kind, run_id, seq, digest, bytes, full_bytes, truncated)
		select 'run://plan/' || g, 'run', 'plan-' || g, 1, md5(g::text), 'x', 1, false
		from generate_series(1, 5000) g;
		analyze run_content;`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rows, err := pool.Query(ctx, "explain "+admin.ExpiredReplaysQuery, time.Now())
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()
	plan := ""
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan: %v", err)
		}
		plan += line + "\n"
	}
	if strings.Contains(plan, "Seq Scan on run_content") {
		t.Errorf("plan reads all the content:\n%s", plan)
	}
}

// A copy inside the window keeps its replays, however old they are themselves: they
// age out as content, and following the copy is only about taking more, never less.
func TestSweep_leavesTheReplaysOfACopyStillInsideTheWindow(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()
	input := []byte(`{"email":"ana@example.com"}`)

	if _, err := content.PutFor(ctx, "regression", "suporte", 1, input); err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	replayRun := fresh("run-replay-")
	replay(t, pool, "suporte", replayRun, "reg-1")
	replayed, err := content.Put(ctx, replayRun, 1, input)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if _, err := erasures.Sweep(ctx); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if _, err := content.Get(ctx, replayed); err != nil {
		t.Fatalf("replay of a copy inside the window: %v", err)
	}
}

// commandCounter counts what reaches the database, including through the fence's
// own connection, which copies this pool's configuration.
type commandCounter struct{ commands atomic.Int64 }

func (c *commandCounter) TraceQueryStart(
	ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData,
) context.Context {
	c.commands.Add(1)
	return ctx
}

func (c *commandCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func countingPool(t *testing.T) (*pgxpool.Pool, *commandCounter) {
	t.Helper()
	config, err := pgxpool.ParseConfig(testdb.DSN(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	counter := &commandCounter{}
	config.ConnConfig.Tracer = counter
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool, counter
}

// Five years of nightly batteries is thousands of replays per case. All of it runs
// holding the fence, so a command per replay is corrections and batteries waiting on
// something proportional to history.
func TestSweep_commandsDoNotGrowWithTheReplaysItTakes(t *testing.T) {
	erasuresFor(t, time.Now)
	seeded := openPool(t)
	pool, counter := countingPool(t)
	erasures := admin.NewErasures(pool, ledger.NewContent(pool),
		admin.NewRetention(pool, settings.NewStore(pool, testVault(t))), regression.NewStore(pool))
	ctx := context.Background()

	measure := func(replays int) int64 {
		t.Helper()
		cleanCorpus(t, seeded)
		content := ledger.NewContent(seeded)
		input := []byte(`{"email":"ana@example.com"}`)
		copied, err := content.PutFor(ctx, "regression", "suporte", 1, input)
		if err != nil {
			t.Fatalf("PutFor: %v", err)
		}
		if _, err := seeded.Exec(ctx, `update run_content set created_at = now() - interval '6 years'
			where ref = $1`, copied); err != nil {
			t.Fatalf("age the copy: %v", err)
		}
		for range replays {
			run := fresh("run-replay-")
			replay(t, seeded, "suporte", run, "reg-1")
			if _, err := content.Put(ctx, run, 1, input); err != nil {
				t.Fatalf("Put: %v", err)
			}
		}
		counter.commands.Store(0)
		if _, err := erasures.Sweep(ctx); err != nil {
			t.Fatalf("Sweep: %v", err)
		}
		return counter.commands.Load()
	}

	// The first pass also pays for a new connection learning the database's types.
	measure(1)
	one, many := measure(1), measure(100)
	if one != many {
		t.Fatalf("commands = %d for one replay, %d for a hundred", one, many)
	}
}

func TestForSubject_commandsDoNotGrowWithTheReplaysItTakes(t *testing.T) {
	erasuresFor(t, time.Now)
	seeded := openPool(t)
	pool, counter := countingPool(t)
	erasures := admin.NewErasures(pool, ledger.NewContent(pool),
		admin.NewRetention(pool, settings.NewStore(pool, testVault(t))), regression.NewStore(pool))
	ctx := context.Background()

	measure := func(replays int) int64 {
		t.Helper()
		cleanCorpus(t, seeded)
		content := ledger.NewContent(seeded)
		input := []byte(`{"email":"ana@example.com"}`)
		copied, err := content.PutFor(ctx, "regression", "suporte", 1, input)
		if err != nil {
			t.Fatalf("PutFor: %v", err)
		}
		recordCorrection(t, seeded, correction{"suporte", "reg-1", "run-asked", copied, ""})
		for range replays {
			run := fresh("run-replay-")
			replay(t, seeded, "suporte", run, "reg-1")
			if _, err := content.Put(ctx, run, 1, input); err != nil {
				t.Fatalf("Put: %v", err)
			}
		}
		counter.commands.Store(0)
		if _, err := erasures.ForSubject(ctx, "dpo", domain.Scope{}, []domain.RunID{"run-asked"}, "request"); err != nil {
			t.Fatalf("ForSubject: %v", err)
		}
		return counter.commands.Load()
	}

	// The first pass also pays for a new connection learning the database's types.
	measure(1)
	one, many := measure(1), measure(100)
	if one != many {
		t.Fatalf("commands = %d for one replay, %d for a hundred", one, many)
	}
}

// A replay holds more than its input: what it did is content too, and it goes whole.
func TestForSubject_takesAllTheContentOfAReplay(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()
	input := []byte(`{"email":"ana@example.com"}`)

	copied, err := content.PutFor(ctx, "regression", "suporte", 1, input)
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	recordCorrection(t, pool, correction{"suporte", "reg-1", "run-asked", copied, ""})
	run := fresh("run-replay-")
	replay(t, pool, "suporte", run, "reg-1")
	if _, err := content.Put(ctx, run, 1, input); err != nil {
		t.Fatalf("Put: %v", err)
	}
	answer, err := content.Put(ctx, run, 5, []byte("the answer quoting ana@example.com"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if _, err := erasures.ForSubject(ctx, "dpo", domain.Scope{}, []domain.RunID{"run-asked"}, "request"); err != nil {
		t.Fatalf("ForSubject: %v", err)
	}

	if _, err := content.Get(ctx, answer); !errors.Is(err, ledger.ErrErased) {
		t.Fatalf("what the replay said: err = %v, want erased", err)
	}
}

// The owner column holds an id of any kind, so a replay's id can be another kind's
// owner too, and that content is not the replay's.
func TestForSubject_leavesAnotherKindOfContentUnderTheSameOwnerId(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()
	input := []byte(`{"email":"ana@example.com"}`)

	copied, err := content.PutFor(ctx, "regression", "suporte", 1, input)
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	recordCorrection(t, pool, correction{"suporte", "reg-1", "run-asked", copied, ""})
	run := fresh("run-replay-")
	replay(t, pool, "suporte", run, "reg-1")
	if _, err := content.Put(ctx, run, 1, input); err != nil {
		t.Fatalf("Put: %v", err)
	}
	unrelated, err := content.PutFor(ctx, "case", string(run), 1, []byte("an uploaded set"))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}

	if _, err := erasures.ForSubject(ctx, "dpo", domain.Scope{}, []domain.RunID{"run-asked"}, "request"); err != nil {
		t.Fatalf("ForSubject: %v", err)
	}

	if _, err := content.Get(ctx, unrelated); err != nil {
		t.Fatalf("content of another kind: %v", err)
	}
}

// A request about one person can name thousands of runs, and the fence is held for
// all of it. Each run here also has a correction and a replay, so nothing about the
// request is smaller for being asked about more runs.
func TestForSubject_commandsDoNotGrowWithTheRunsItNames(t *testing.T) {
	erasuresFor(t, time.Now)
	seeded := openPool(t)
	pool, counter := countingPool(t)
	erasures := admin.NewErasures(pool, ledger.NewContent(pool),
		admin.NewRetention(pool, settings.NewStore(pool, testVault(t))), regression.NewStore(pool))
	ctx := context.Background()

	measure := func(named int) int64 {
		t.Helper()
		cleanCorpus(t, seeded)
		content := ledger.NewContent(seeded)
		runs := make([]domain.RunID, 0, named)
		for at := range named {
			// Different on every pass: identical bytes would land on the last pass's
			// tombstone, and there would be no copy left to count.
			run := fresh("run-named-")
			input := []byte(fmt.Sprintf(`{"email":"person-%d@example.com","run":%q}`, at, run))
			runs = append(runs, run)
			inputRef, err := content.Put(ctx, run, 1, input)
			if err != nil {
				t.Fatalf("Put: %v", err)
			}
			open(t, seeded, "suporte", run, domain.RunStartedPayload{Trigger: "manual", InputRef: inputRef})
			copied, err := content.PutFor(ctx, "regression", "suporte", 1, input)
			if err != nil {
				t.Fatalf("PutFor: %v", err)
			}
			recordCorrection(t, seeded, correction{"suporte", fmt.Sprintf("reg-%d", at), string(run), copied, "note"})
			replayRun := fresh("run-replay-")
			replay(t, seeded, "suporte", replayRun, fmt.Sprintf("reg-%d", at))
			if _, err := content.Put(ctx, replayRun, 1, input); err != nil {
				t.Fatalf("Put: %v", err)
			}
		}
		counter.commands.Store(0)
		erased, err := erasures.ForSubject(ctx, "dpo", domain.Scope{}, runs, "request")
		if err != nil {
			t.Fatalf("ForSubject: %v", err)
		}
		// Own input, the copy and the replay's input, for each run named.
		if erased != 3*named {
			t.Fatalf("erased = %d objects for %d runs, want %d", erased, named, 3*named)
		}
		return counter.commands.Load()
	}

	// The first pass also pays for a new connection learning the database's types.
	measure(1)
	one, many := measure(1), measure(100)
	if one != many {
		t.Fatalf("commands = %d for one named run, %d for a hundred", one, many)
	}
}

// One statement over many runs must still stop at those runs, and at this kind.
func TestForSubject_namingManyRuns_leavesOtherRunsAndKindsAlone(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()

	first, second, other := fresh("run-a-"), fresh("run-b-"), fresh("run-c-")
	refs := make([]string, 0, 2)
	for _, run := range []domain.RunID{first, second} {
		ref, err := content.Put(ctx, run, 1, []byte("named "+string(run)))
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		refs = append(refs, ref)
	}
	kept, err := content.Put(ctx, other, 1, []byte("not named"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	sameOwner, err := content.PutFor(ctx, "case", string(first), 1, []byte("an uploaded set"))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}

	erased, err := erasures.ForSubject(ctx, "dpo", domain.Scope{}, []domain.RunID{first, second}, "request")
	if err != nil {
		t.Fatalf("ForSubject: %v", err)
	}
	if erased != 2 {
		t.Fatalf("erased = %d, want 2", erased)
	}

	for _, ref := range refs {
		if _, err := content.Get(ctx, ref); !errors.Is(err, ledger.ErrErased) {
			t.Errorf("a named run's content: err = %v, want erased", err)
		}
	}
	// The tombstone says why, and an erasure nobody can account for looks like loss.
	var reason string
	if err := pool.QueryRow(ctx, `select erased_reason from run_content where ref = $1`,
		refs[0]).Scan(&reason); err != nil || reason != "subject" {
		t.Errorf("reason = %q, %v; want subject", reason, err)
	}
	for name, ref := range map[string]string{"a run nobody named": kept, "another kind under that id": sameOwner} {
		if _, err := content.Get(ctx, ref); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
