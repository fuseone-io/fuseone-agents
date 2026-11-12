package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/drift"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
	"github.com/fuseone/agents/internal/testdb"
)

/*
The drift watcher, from the ledger to the notice.

Its own tests hand it reports with Unmet already filled in, and nothing in production ever
did that: only a battery read against its corpus knows what stopped holding. So for months
this loop compared two lists of runs with nothing wrong in them and said nothing. Here
nothing is prefabricated. The runs are in the ledger, and the corrections in the corpus.
*/

type heardNotice struct{ posted []channel.Message }

func (h *heardNotice) Announce(_ context.Context, _ domain.Scope, m channel.Message) error {
	h.posted = append(h.posted, m)
	return nil
}

type driftWorld struct {
	pool    *pgxpool.Pool
	store   *ledger.Postgres
	corpora *regression.Store
	opened  time.Time
}

func newDriftWorld(t *testing.T) *driftWorld {
	t.Helper()
	pool := testdb.Isolated(t)
	corpora := regression.NewStore(pool)
	content := ledger.NewContent(pool)
	ctx := t.Context()
	copied, err := content.RestoreFor(ctx, "regression", "triage", 1, []byte(`{"ask":"refund"}`))
	if err != nil {
		t.Fatalf("keep the occurrence: %v", err)
	}
	if err := corpora.Record(ctx, domain.RegressionCase{
		ID: "estorno", Agent: "triage", Scope: domain.Scope{Company: "acme", Area: "cx"},
		InputRef: copied, FromRun: "run-origin",
		Expectations: []domain.Expectation{{Kind: domain.ExpectCalls, Value: "crm.lookup"}},
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	// A published version, as far as the state can tell: claimed with a
	// digest, which the database requires of a current version.
	if _, err := pool.Exec(ctx, `
		insert into agent_version_digests (agent_id, version_id, digest)
		values ('triage', 'v2', repeat('2', 64))`); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into agent_state (agent_id, current_version) values ('triage', 'v2')`); err != nil {
		t.Fatalf("publish: %v", err)
	}
	return &driftWorld{
		pool: pool, store: ledger.NewPostgres(pool), corpora: corpora,
		opened: time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC),
	}
}

// battery writes one settled run of the case. Reaching the tool is what holds it.
func (w *driftWorld) battery(t *testing.T, simulation string, pin int64, reached bool) {
	t.Helper()
	run := domain.RunID("run-" + simulation)
	w.opened = w.opened.Add(time.Hour)
	w.append(t, domain.Step{RunID: run, Seq: 1, Kind: domain.StepRunStarted, IdemKey: string(run)},
		domain.RunStartedPayload{
			Trigger: "simulation", Simulated: true, Simulation: simulation, Case: "estorno",
			DatasetVersion: pin,
		})
	seq := int64(2)
	if reached {
		w.append(t, domain.Step{RunID: run, Seq: seq, Kind: domain.StepGateDecided},
			domain.GateDecidedPayload{Tool: "crm.lookup", Effect: domain.EffectRead, Verdict: domain.VerdictAllow})
		w.append(t, domain.Step{RunID: run, Seq: seq + 1, Kind: domain.StepToolCalled}, domain.ToolCalledPayload{})
		seq += 2
	}
	w.append(t, domain.Step{RunID: run, Seq: seq, Kind: domain.StepRunFinished},
		domain.RunFinishedPayload{Outcome: "answered"})
}

func (w *driftWorld) append(t *testing.T, step domain.Step, payload any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	step.AgentID, step.VersionID, step.Payload, step.At = "triage", "v2", raw, w.opened
	step.Scope = domain.Scope{Company: "acme", Area: "cx"}
	if _, err := w.store.Append(t.Context(), step); err != nil {
		t.Fatalf("append %s: %v", step.Kind, err)
	}
}

func (w *driftWorld) sweep(t *testing.T) []channel.Message {
	t.Helper()
	heard := &heardNotice{}
	watch := drift.New(w.corpora, w.store, folded{w.store, w.corpora}, heard, nil)
	if _, err := watch.Sweep(t.Context(), w.opened); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	return heard.posted
}

func TestDriftWatcher_aCorrectionThatStoppedHolding_isAnnounced(t *testing.T) {
	world := newDriftWorld(t)
	world.battery(t, "sim-1", 1, true)
	world.battery(t, "sim-2", 1, false)

	posted := world.sweep(t)

	if len(posted) != 1 || posted[0].Event != channel.EventDrifted {
		t.Fatalf("posted = %+v, want one drift notice", posted)
	}
}

func TestDriftWatcher_stillHolding_saysNothing(t *testing.T) {
	world := newDriftWorld(t)
	world.battery(t, "sim-1", 1, true)
	world.battery(t, "sim-2", 1, true)

	if posted := world.sweep(t); len(posted) != 0 {
		t.Errorf("posted = %+v, want nothing", posted)
	}
}

// A correction that came back is good news, and it is not what this says.
func TestDriftWatcher_aCorrectionThatCameBack_saysNothing(t *testing.T) {
	world := newDriftWorld(t)
	world.battery(t, "sim-1", 1, false)
	world.battery(t, "sim-2", 1, true)

	if posted := world.sweep(t); len(posted) != 0 {
		t.Errorf("posted = %+v, want nothing", posted)
	}
}

// Somebody changed what it expects, and it broke. That is the author's move, not the
// world moving under them, so it must not read as drift.
func TestDriftWatcher_aCaseWhoseExpectationsChanged_saysNothing(t *testing.T) {
	world := newDriftWorld(t)
	world.battery(t, "sim-1", 1, true)
	ctx := t.Context()
	cases, err := world.corpora.List(ctx, "triage")
	if err != nil || len(cases) != 1 {
		t.Fatalf("List: %+v, %v", cases, err)
	}
	changed := cases[0]
	changed.Expectations = []domain.Expectation{{Kind: domain.ExpectCalls, Value: "crm.other"}}
	if err := world.corpora.Record(ctx, changed); err != nil {
		t.Fatalf("Record: %v", err)
	}
	world.battery(t, "sim-2", 2, false)

	if posted := world.sweep(t); len(posted) != 0 {
		t.Errorf("posted = %+v, want nothing", posted)
	}
}

// Batteries from before versions have no pin, and must still be judged.
func TestDriftWatcher_unpinnedBatteries_areStillJudged(t *testing.T) {
	world := newDriftWorld(t)
	world.battery(t, "sim-1", 0, true)
	world.battery(t, "sim-2", 0, false)

	if posted := world.sweep(t); len(posted) != 1 {
		t.Errorf("posted = %+v, want one drift notice", posted)
	}
}

// Both batteries ran version 1, and the second broke it. That the corpus has since moved on
// does not change what happened: read against the corpus as it is, both would look broken
// and nothing would have changed.
func TestDriftWatcher_theCorpusMovedSince_stillReadsBothAgainstWhatTheyRan(t *testing.T) {
	world := newDriftWorld(t)
	world.battery(t, "sim-1", 1, true)
	world.battery(t, "sim-2", 1, false)
	ctx := t.Context()
	cases, err := world.corpora.List(ctx, "triage")
	if err != nil || len(cases) != 1 {
		t.Fatalf("List: %+v, %v", cases, err)
	}
	changed := cases[0]
	changed.Expectations = []domain.Expectation{{Kind: domain.ExpectCalls, Value: "crm.other"}}
	if err := world.corpora.Record(ctx, changed); err != nil {
		t.Fatalf("Record: %v", err)
	}

	if posted := world.sweep(t); len(posted) != 1 {
		t.Errorf("posted = %+v, want one drift notice", posted)
	}
}
