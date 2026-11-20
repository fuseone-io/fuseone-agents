package trigger_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/trigger"
)

/*
The opener accepts a candidate only for an experiment's simulated run (NT-018 phase 2,
release 2b).

Any other request naming a seal gets the error a version the agent never published gets,
so a candidate's id says nothing. An experiment on a candidate needs no published version,
runs in the sealed scope, and is refused only by a pause the agent actually has (an agent
never published has nothing to pause) and by a stop of the installation, the sealed scope
or the agent.
*/

const (
	candidateSeal  = "0b7c3e52-8a43-4b55-9a3f-0d6a3c5f9e11"
	candidateDraft = "1c8d4f63-9b54-4c66-8a4f-1e7b4d6a0f22"
)

var sealedScope = domain.Scope{Company: "acme", Area: "finance"}

type candidates map[string]trigger.Candidate

func (c candidates) Candidate(_ context.Context, agent domain.AgentID, seal string) (trigger.Candidate, bool, error) {
	got, ok := c[seal]
	if !ok || got.Agent != agent {
		return trigger.Candidate{}, false, nil
	}
	return got, true, nil
}

// candidateOpener is an opener for an agent published only in cx, with one candidate
// sealed in finance.
func candidateOpener(t *testing.T, paused bool) (*trigger.Opener, *ledger.Memory) {
	t.Helper()
	store := ledger.NewMemory()
	reg := registry{versions: []domain.AgentSummary{{
		ID: "triage", VersionID: "v2", Scope: domain.Scope{Company: "acme", Area: "cx"}, Latest: true,
	}}}
	clock := fixedClock{t: time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)}
	return trigger.NewOpener(store, reg, clock).WithContent(engine.NewMemoryContent()).
		WithCandidates(candidates{candidateSeal: {
			Agent: "triage", Version: "vcandidate01", Scope: sealedScope, Paused: paused, DraftID: candidateDraft,
		}}), store
}

func experimentOnSeal(key string) trigger.Request {
	return trigger.Request{
		Agent: "triage", IdemKey: key, Trigger: "simulation", Simulation: "exp_triage_1",
		Case: "case-1", Experiment: "exp_triage_1", SealID: candidateSeal,
	}
}

func TestOpen_anExperimentOnACandidate_runsTheSealedVersionInTheSealedScope(t *testing.T) {
	t.Parallel()
	opener, store := candidateOpener(t, false)

	got, err := opener.Open(t.Context(), experimentOnSeal("cand-1"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	steps, err := store.Read(context.Background(), got.RunID, domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var started domain.RunStartedPayload
	if err := json.Unmarshal(steps[0].Payload, &started); err != nil {
		t.Fatal(err)
	}
	if steps[0].VersionID != "vcandidate01" || steps[0].Scope != sealedScope || got.Scope != sealedScope ||
		started.SealID != candidateSeal || !started.Simulated || started.Experiment != "exp_triage_1" {
		t.Fatalf("opened %s in %s with %+v, want the candidate in the sealed scope", steps[0].VersionID, steps[0].Scope, started)
	}
	again, err := opener.Open(t.Context(), experimentOnSeal("cand-1"))
	if err != nil || again.RunID != got.RunID || again.Created {
		t.Errorf("the same key again = %+v (%v), want the run it opened", again, err)
	}
}

func TestOpen_anyOtherRequestNamingASeal_isAnUnknownVersion(t *testing.T) {
	t.Parallel()
	real := experimentOnSeal("real")
	real.Trigger, real.Simulation, real.Experiment = "manual", "", ""
	battery := experimentOnSeal("battery")
	battery.Experiment = ""
	otherAgent := experimentOnSeal("other-agent")
	otherAgent.Agent = "billing"
	otherVersion := experimentOnSeal("other-version")
	otherVersion.Version = "v2"
	unknown := experimentOnSeal("unknown")
	unknown.SealID = "9f9f9f9f-8a43-4b55-9a3f-0d6a3c5f9e11"
	for name, req := range map[string]trigger.Request{
		"a real run": real, "a battery run": battery, "a seal of another agent": otherAgent,
		"a version the seal does not hold": otherVersion, "a seal nobody wrote": unknown,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opener, _ := candidateOpener(t, false)
			if _, err := opener.Open(t.Context(), req); !errors.Is(err, trigger.ErrUnknownVersion) {
				t.Fatalf("err = %v, want ErrUnknownVersion", err)
			}
		})
	}
	t.Run("an opener that cannot read seals", func(t *testing.T) {
		t.Parallel()
		opener, _ := openerFor(t)
		if _, err := opener.Open(t.Context(), experimentOnSeal("unwired")); !errors.Is(err, trigger.ErrUnknownVersion) {
			t.Fatalf("err = %v, want ErrUnknownVersion", err)
		}
	})
}

// Decision 3: a candidate is refused only when the agent's state says paused. An agent
// never published has no state, and the opener's ordinary reading of that as paused must
// not refuse its first experiment.
func TestOpen_aCandidate_isRefusedOnlyByAPauseTheAgentHas(t *testing.T) {
	t.Parallel()
	opener, _ := candidateOpener(t, true)
	if _, err := opener.WithPauses(paused{}).Open(t.Context(), experimentOnSeal("paused")); !errors.Is(err, trigger.ErrPaused) {
		t.Errorf("a paused agent: err = %v, want ErrPaused", err)
	}

	running, _ := candidateOpener(t, false)
	if _, err := running.WithPauses(paused{}).Open(t.Context(), experimentOnSeal("never-published")); err != nil {
		t.Errorf("an agent with no state: %v, want the experiment opened", err)
	}
}

func TestOpen_aCandidate_isStoppedByTheInstallationTheSealedScopeOrTheAgent(t *testing.T) {
	t.Parallel()
	for name, stop := range map[string]domain.Stop{
		"the installation": {Level: domain.StopInstallation, Reason: "incident"},
		"the sealed scope": {Level: domain.StopScope, Scope: sealedScope, Reason: "finance halt"},
		"the agent":        {Level: domain.StopAgent, Agent: "triage", Reason: "agent halt"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opener, _ := candidateOpener(t, false)
			if _, err := opener.WithStops(stops{stop}).Open(t.Context(), experimentOnSeal("stopped")); !errors.Is(err, trigger.ErrStopped) {
				t.Fatalf("err = %v, want ErrStopped", err)
			}
		})
	}
	t.Run("the published scope alone", func(t *testing.T) {
		t.Parallel()
		opener, _ := candidateOpener(t, false)
		cx := domain.Stop{Level: domain.StopScope, Scope: domain.Scope{Company: "acme", Area: "cx"}, Reason: "cx halt"}
		if _, err := opener.WithStops(stops{cx}).Open(t.Context(), experimentOnSeal("cx-stopped")); err != nil {
			t.Fatalf("a stop of the agent's published scope stopped a run in the sealed one: %v", err)
		}
	})
}
