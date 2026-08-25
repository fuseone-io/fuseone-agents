package simulate_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/simulate"
	"github.com/fuseone/agents/internal/trigger"
)

type registry struct{}

func (registry) Versions(context.Context, domain.AgentID) ([]domain.AgentSummary, error) {
	return []domain.AgentSummary{{
		ID: "suporte", VersionID: "v1", Latest: true,
		Scope: domain.Scope{Company: "acme", Area: "cx"},
	}}, nil
}

type catalog map[domain.ToolID]domain.Effect

func (c catalog) Effect(id domain.ToolID) (domain.Effect, bool) {
	e, ok := c[id]
	return e, ok
}

func (c catalog) Dedupe(domain.ToolID) (domain.ToolDedupe, bool) {
	return domain.ToolDedupe{}, false
}

type clock struct{}

func (clock) Now() time.Time { return time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC) }

// onceThenDone proposes one call per run and then reports the run complete.
type onceThenDone struct{ tool domain.ToolID }

func (p onceThenDone) Plan(_ context.Context, in engine.PlanInput) (engine.Proposal, error) {
	if len(in.State.Called) == 0 {
		return engine.Proposal{Tool: p.tool, Args: []byte(`{}`)}, nil
	}
	return engine.Proposal{Done: true, Outcome: "respondido"}, nil
}

type failsOn struct {
	inner *trigger.Opener
	at    int
	seen  int
}

func (f *failsOn) Open(ctx context.Context, req trigger.Request) (trigger.Result, error) {
	f.seen++
	if f.seen == f.at {
		return trigger.Result{}, errors.New("trigger: the agent is paused")
	}
	return f.inner.Open(ctx, req)
}

func openerFor(store *ledger.Memory, content engine.ContentStore) *trigger.Opener {
	return trigger.NewOpener(store, registry{}, clock{}).WithContent(content)
}

func depsFor(store *ledger.Memory, content *engine.MemoryContent, tools engine.Tools) engine.Deps {
	return engine.Deps{
		Ledger: store, Content: content, Gate: gate.New(), Tools: tools,
		Catalog: catalog{"crm.lookup": domain.EffectRead, "crm.refund": domain.EffectFinancial},
		Clock:   clock{},
	}
}

func batchOf(cases ...string) simulate.Batch {
	inputs := make([]simulate.Occurrence, 0, len(cases))
	for _, c := range cases {
		inputs = append(inputs, simulate.Occurrence{Input: []byte(c)})
	}
	return simulate.Batch{ID: "sim-1", Agent: "suporte", By: "ana", Cases: inputs}
}

func TestOpen_opensOneRunPerCase_eachNamingItsSimulation(t *testing.T) {
	t.Parallel()

	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	got, err := simulate.Open(t.Context(), openerFor(store, content),
		batchOf(`{"n":1}`, `{"n":2}`, `{"n":3}`))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(got.Runs) != 3 || len(got.Failed) != 0 {
		t.Fatalf("opened = %+v", got)
	}

	seen := map[domain.RunID]bool{}
	for i, id := range got.Runs {
		if seen[id] {
			// Three cases sharing a run is three cases reported as one.
			t.Fatalf("case %d reuses run %q", i+1, id)
		}
		seen[id] = true

		steps, err := store.Read(t.Context(), id, domain.FirstSeq)
		if err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		var p domain.RunStartedPayload
		if err := json.Unmarshal(steps[0].Payload, &p); err != nil {
			t.Fatalf("payload: %v", err)
		}
		// The mark and the batch travel in the step, so every projection
		// excludes it and the report can find it again.
		if !p.Simulated || p.Simulation != "sim-1" || p.Trigger != "simulation" {
			t.Errorf("run %s opened as %+v", id, p)
		}
	}
}

func TestOpen_aCaseThatCannotOpen_isCountedAndNamed(t *testing.T) {
	t.Parallel()

	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	opener := &failsOn{inner: openerFor(store, content), at: 2}

	got, err := simulate.Open(t.Context(), opener, batchOf(`{"n":1}`, `{"n":2}`, `{"n":3}`))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// One case failing never stops the rest, and the ones that did not open
	// are named rather than dropped: forty-eight runs from fifty cases is a
	// set of forty-eight, and saying fifty is coverage that lies.
	if len(got.Runs) != 2 {
		t.Errorf("runs = %d, want the two that opened", len(got.Runs))
	}
	if len(got.Failed) != 1 {
		t.Fatalf("failed = %v, want the one that did not", got.Failed)
	}
}

func TestOpen_theCasesAreWhatTheRunsAreAbout(t *testing.T) {
	t.Parallel()

	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	got, err := simulate.Open(t.Context(), openerFor(store, content), batchOf(`{"assunto":"cobrança"}`))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	steps, _ := store.Read(t.Context(), got.Runs[0], domain.FirstSeq)
	var p domain.RunStartedPayload
	if err := json.Unmarshal(steps[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	// Outside the ledger like any other input: a real occurrence carries
	// whatever it carries, including personal data (AU-04).
	body, err := content.Get(t.Context(), p.InputRef)
	if err != nil {
		t.Fatalf("Get(%q): %v", p.InputRef, err)
	}
	if string(body) != `{"assunto":"cobrança"}` {
		t.Errorf("input = %s", body)
	}
}
