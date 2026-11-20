package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/worker"
)

/*
A run opened on a candidate, through the simulation pool the worker command builds
(NT-018 phase 2, release 2a).

Release 2b writes seals and opens these runs; this release only has to read them. So the
journey writes the claim, the bytes and the row the way 2b will, opens the run in the
ledger, and lets the production pool resolve it. A pool that was never handed the seal
reader fails these runs with the same code an unreadable seal does, so only a run that
plans on the sealed model proves the wiring.
*/

const candidateTemplate = `---
id: %s
name: Triage
company: acme
area: cx
provider: elsewhere
model: sealed-model
tools:
  - crm.lookup
budget:
  micros: 500000
  steps: 20
---

Classify the ticket from the sealed text.
`

// candidateBytes is the sealed definition of agent, naming a provider only it uses.
func candidateBytes(agent domain.AgentID) string { return fmt.Sprintf(candidateTemplate, agent) }

// seal writes a candidate of the world's agent the way release 2b will.
func (w *experimentWorld) seal(t *testing.T, sealID string) domain.VersionID {
	t.Helper()
	return w.sealFrom(t, sealID, "")
}

// sealFrom seals the candidate from a draft, or from a draft nobody has when draftID is empty.
func (w *experimentWorld) sealFrom(t *testing.T, sealID, draftID string) domain.VersionID {
	t.Helper()
	return w.sealDefinition(t, sealID, draftID, candidateBytes(w.agent))
}

// sealDefinition seals a definition from a draft, the way the seal route writes one.
func (w *experimentWorld) sealDefinition(t *testing.T, sealID, draftID, text string) domain.VersionID {
	t.Helper()
	sealed, err := spec.Parse("triage.agent.md", []byte(text))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	sum := sha256.Sum256([]byte(text))
	ctx := t.Context()
	if _, err := w.pool.Exec(ctx, `
		insert into agent_version_digests (agent_id, version_id, digest) values ($1, $2, $3)
		on conflict do nothing`, string(w.agent), string(sealed.Version), hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("claim: %v", err)
	}
	ref, err := ledger.NewContent(w.pool).PutFor(ctx, "authoring", sealID, 0, []byte(text))
	if err != nil {
		t.Fatalf("keep the sealed bytes: %v", err)
	}
	if _, err := w.pool.Exec(ctx, `
		insert into agent_seals (seal_id, agent_id, version_id, company_id, area_id,
		  content_ref, draft_id, draft_revision, sealed_by)
		values ($1::uuid, $2, $3, 'acme', 'cx', $4, coalesce(nullif($5, '')::uuid, gen_random_uuid()), 'rev-1', 'usr_ana')`,
		sealID, string(w.agent), string(sealed.Version), ref, draftID); err != nil {
		t.Fatalf("seal: %v", err)
	}
	return sealed.Version
}

// openOnSeal opens a simulated experiment run pinned to a seal.
func (w *experimentWorld) openOnSeal(t *testing.T, run domain.RunID, version domain.VersionID, sealID string) {
	t.Helper()
	payload, err := json.Marshal(domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: "exp-cand", Case: "refund",
		Experiment: "exp-cand", SealID: sealID,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := w.store.Append(t.Context(), domain.Step{
		RunID: run, Seq: 1, Kind: domain.StepRunStarted, Scope: domain.Scope{Company: "acme", Area: "cx"},
		AgentID: w.agent, VersionID: version, At: time.Now(), IdemKey: string(run) + ":1", Payload: payload,
	}); err != nil {
		t.Fatalf("open: %v", err)
	}
}

// settleCandidate runs the production simulation pool, with the database it is given in
// production, until the run reaches an ending.
func (w *experimentWorld) settleCandidate(t *testing.T, run domain.RunID) engine.State {
	t.Helper()
	return w.advanceUntil(t, run, func(s engine.State) bool {
		return s.Phase == engine.PhaseFinished || s.Phase == engine.PhaseFailed
	})
}

// advanceUntil runs the production simulation pool until the run's state satisfies done.
func (w *experimentWorld) advanceUntil(t *testing.T, run domain.RunID, done func(engine.State) bool) engine.State {
	t.Helper()
	parts := &workerParts{store: w.store, queue: w.store, catalog: w.catalog, configPool: w.pool}
	deps := engine.Deps{Ledger: w.store, Gate: gate.New(), Tools: w.catalog, Catalog: w.catalog,
		Clock: engine.SystemClock{}, Content: ledger.NewContent(w.pool)}
	_, sim := parts.pools(workerFlags{owner: "candidate", concurrency: 1, lease: time.Minute},
		deps, w.resolver, worker.NewMetricsRegistry())
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		if err := sim.Run(ctx); err != nil && ctx.Err() == nil {
			t.Errorf("the simulation pool stopped: %v", err)
		}
	}()
	defer func() { cancel(); <-stopped }()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		steps, err := w.store.Read(t.Context(), run, domain.FirstSeq)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		state, err := engine.Fold(steps)
		if err != nil {
			t.Fatalf("Fold: %v", err)
		}
		if done(state) {
			return state
		}
	}
	t.Fatalf("the run never reached the state awaited")
	return engine.State{}
}

func TestCandidate_aReadableSeal_runsOnTheSealedDefinitionInTheProductionPool(t *testing.T) {
	world := newExperimentWorld(t)
	const sealID = "5a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"
	version := world.seal(t, sealID)
	world.openOnSeal(t, "run-cand-1", version, sealID)

	state := world.settleCandidate(t, "run-cand-1")

	if state.Phase != engine.PhaseFinished {
		t.Fatalf("phase = %v, want finished", state.Phase)
	}
	if got := world.tested.models(); !slices.Equal(got, []string{"sealed-model"}) {
		t.Errorf("the sealed provider was asked %v, want the sealed model once", got)
	}
	if got := world.declared.models(); len(got) != 0 {
		t.Errorf("the published version's provider was asked %v, want nothing", got)
	}
}

func TestCandidate_anErasedSeal_failsTheRunWithoutAskingAModel(t *testing.T) {
	world := newExperimentWorld(t)
	const sealID = "6b2c3d4e-5f60-4b7c-9d8e-0f1a2b3c4d5e"
	version := world.seal(t, sealID)
	tx, err := world.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.NewContent(world.pool).EraseOwnersTx(t.Context(), tx, "authoring", []string{sealID}, "test"); err != nil {
		t.Fatalf("erase the seal: %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	world.openOnSeal(t, "run-cand-2", version, sealID)

	state := world.settleCandidate(t, "run-cand-2")

	if state.Phase != engine.PhaseFailed {
		t.Fatalf("phase = %v, want failed", state.Phase)
	}
	if asked := append(world.tested.models(), world.declared.models()...); len(asked) != 0 {
		t.Errorf("models asked %v, want none", asked)
	}
	steps, err := world.store.Read(t.Context(), "run-cand-2", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var failed domain.FailedPayload
	if err := json.Unmarshal(steps[len(steps)-1].Payload, &failed); err != nil ||
		failed.Code != worker.CodeCandidateUnreadable {
		t.Errorf("last step %s %s, want the candidate-unreadable failure", steps[len(steps)-1].Kind, steps[len(steps)-1].Payload)
	}
}
