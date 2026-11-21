package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
)

/*
A battery's or an experiment's run of a case replays the world the case was recorded with
(NT-018 phase 5). The worker reads it from the version of the corpus the run's opening names,
and only for a simulated run of a case of a version: never from the model, never for a
rehearsal, which waits for its author instead. A world that cannot be read fails the run with
fuseone:world-unreadable before anything plans, and so does a pool that cannot read worlds:
replaying the case without its world would report it held on a situation nobody corrected.
*/

type recordingWorlds struct {
	asked []string
	world *domain.CaseWorld
	err   error
}

func (w *recordingWorlds) WorldOf(_ context.Context, agent domain.AgentID, version int64, caseID string) (*domain.CaseWorld, error) {
	w.asked = append(w.asked, fmt.Sprintf("%s@%d/%s", agent, version, caseID))
	return w.world, w.err
}

const worldCallArgs = "sha256:abc"

func aCaseOfVersion() domain.RunStartedPayload {
	return domain.RunStartedPayload{Trigger: "simulation", Simulated: true, Simulation: "sim-1", Case: "reg-3", DatasetVersion: 2}
}

// worldTurn opens run-1 with started and a call left pending, then runs one turn of a
// simulation pool with worlds wired when given.
func worldTurn(t *testing.T, started domain.RunStartedPayload, worlds Worlds) *ledger.Memory {
	t.Helper()
	store := ledger.NewMemory()
	openSimulated(t, store, started)
	for _, s := range []domain.Step{
		{Kind: domain.StepBudgetReserved, Payload: mustPayload(t, domain.BudgetReservedPayload{Micros: 1000})},
		{Kind: domain.StepToolCalled, Payload: mustPayload(t, domain.ToolCalledPayload{Tool: "crm.lookup", Effect: domain.EffectRead, ArgsDigest: worldCallArgs})},
	} {
		s.RunID, s.Scope, s.At = "run-1", domain.Scope{Company: "acme", Area: "cx"}, time.Now()
		if _, err := store.Append(context.Background(), s); err != nil {
			t.Fatalf("seed %s: %v", s.Kind, err)
		}
	}
	clock := frozenClock{t: time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)}
	deps := engine.Deps{Ledger: store, Content: engine.NewMemoryContent(), Gate: gate.New(), Tools: noTools{}, Catalog: emptyCatalog{}, Clock: clock}
	specs := &experimentSpecs{planner: &flakyPlanner{}}
	w := New(Config{Owner: "w-sim"}, store.Simulations(), deps, specs, clock, slog.New(slog.DiscardHandler)).
		WithStages(stageIs(domain.StageDraft)).WithExperiments(specs).
		WithCandidates(&recordingCandidates{planner: &flakyPlanner{}})
	if worlds != nil {
		w = w.WithWorlds(worlds)
	}
	if claimed, err := w.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil || !claimed {
		t.Fatalf("turn: claimed=%v err=%v", claimed, err)
	}
	return store
}

func replayedPositions(t *testing.T, store *ledger.Memory) []int {
	t.Helper()
	steps, _ := store.Read(context.Background(), "run-1", domain.FirstSeq)
	var out []int
	for _, s := range steps {
		if s.Kind != domain.StepToolReturned {
			continue
		}
		var p domain.ToolReturnedPayload
		if err := json.Unmarshal(s.Payload, &p); err != nil {
			t.Fatalf("decode: %v", err)
		}
		out = append(out, p.Replayed)
	}
	return out
}

func TestTurn_aCaseOfAVersion_replaysTheWorldItsOpeningNames(t *testing.T) {
	t.Parallel()
	worlds := &recordingWorlds{world: &domain.CaseWorld{Case: "reg-3", Entries: []domain.WorldEntry{
		{Position: 1, Tool: "crm.lookup", ArgsDigest: worldCallArgs, AnswerRef: "case_world://agent/reg-3/2/x"},
	}}}
	store := worldTurn(t, aCaseOfVersion(), worlds)
	if len(worlds.asked) != 1 || worlds.asked[0] != "triage@2/reg-3" {
		t.Errorf("asked %v, want the case at the version the opening names", worlds.asked)
	}
	if got := replayedPositions(t, store); len(got) != 1 || got[0] != 1 {
		t.Errorf("replayed %v, want the pending call answered from entry 1", got)
	}
}

func TestTurn_aWorldThatCannotBeRead_failsTheRunBeforeAnythingPlans(t *testing.T) {
	t.Parallel()
	for name, worlds := range map[string]Worlds{
		"the world is unreadable":        &recordingWorlds{err: fmt.Errorf("gone: %w", domain.ErrWorldUnreadable)},
		"the pool cannot read any world": nil,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store := worldTurn(t, aCaseOfVersion(), worlds)
			if code := failedCode(t, store); code != CodeWorldUnreadable {
				t.Errorf("failed with %q, want %q", code, CodeWorldUnreadable)
			}
		})
	}
}

// Only a simulated run of a case of a version replays one: a rehearsal waits for its author,
// and a run of cases written for one simulation names no version to read a world from.
func TestTurn_aRunThatIsNotACaseOfAVersion_readsNoWorld(t *testing.T) {
	t.Parallel()
	for name, started := range map[string]domain.RunStartedPayload{
		"a rehearsal":             {Trigger: "rehearsal", Simulated: true, Simulation: "reh-1", Rehearsal: true, SealID: testSeal, Case: "reg-3", DatasetVersion: 2},
		"cases of one simulation": {Trigger: "simulation", Simulated: true, Simulation: "sim-1", Case: "case-1"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			worlds := &recordingWorlds{err: domain.ErrWorldUnreadable}
			store := worldTurn(t, started, worlds)
			if len(worlds.asked) != 0 {
				t.Errorf("asked %v, want no world read", worlds.asked)
			}
			if head, _ := store.Head(context.Background(), "run-1"); head.Kind == domain.StepFailed {
				t.Error("the run failed, want it to go on without a world")
			}
		})
	}
}
