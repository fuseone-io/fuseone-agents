package main

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
)

// seedDemo drives the real loop against stub collaborators so the development
// server has runs to serve. Nothing here is a fixture: every step goes through
// the Gate and lands in the ledger exactly as production would write it.
func seedDemo(ctx context.Context, store Store, content engine.ContentStore) error {
	// Anchored a few hours in the past rather than at a fixed date: the console
	// shows relative times, and a hardcoded instant reads as "in 6 hours" the
	// moment the wall clock passes it. Still deterministic within a process,
	// which is all the demo needs.
	base := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Minute)

	scenarios := []struct {
		runID     domain.RunID
		agent     domain.AgentID
		area      domain.AreaID
		proposals []engine.Proposal
	}{
		{
			runID: "run-triage-8801", agent: "ticket-triage", area: "cx",
			proposals: []engine.Proposal{
				{Tool: "crm.lookup", Args: []byte(`{"email":"cliente@exemplo.com.br"}`),
					Estimate: cons(40_000), Cost: cost(1_240, 210, 9_800)},
				{Tool: "kb.search", Args: []byte(`{"q":"segunda via boleto"}`),
					Estimate: cons(30_000), Cost: cost(2_100, 180, 7_400)},
				{Done: true, Outcome: "classified", Cost: cost(900, 120, 4_100)},
			},
		},
		{
			// Stops awaiting a human: a write is proposed and the Gate suspends it.
			runID: "run-triage-8802", agent: "ticket-triage", area: "cx",
			proposals: []engine.Proposal{
				{Tool: "crm.lookup", Args: []byte(`{"email":"outro@exemplo.com.br"}`),
					Estimate: cons(40_000), Cost: cost(1_180, 200, 9_300)},
				{Tool: "crm.note", Args: []byte(`{"text":"cobranca duplicada"}`),
					Estimate: cons(25_000), Cost: cost(1_050, 260, 8_600)},
			},
		},
		{
			// Parks: the ceiling is far below what the call would reserve.
			runID: "run-leads-4410", agent: "lead-qualifier", area: "marketing",
			proposals: []engine.Proposal{
				{Tool: "crm.lookup", Args: []byte(`{"segment":"enterprise"}`),
					Estimate: cons(900_000), Cost: cost(3_400, 410, 15_200)},
			},
		},
	}

	for i, sc := range scenarios {
		budget := domain.Budget{Micros: 500_000, ToolCalls: 20, Steps: 60}
		if sc.runID == "run-leads-4410" {
			budget.Micros = 20_000
		}

		runner := engine.NewRunner(engine.Deps{
			Ledger:  store,
			Content: content,
			Gate:    gate.New(),
			Planner: &stubPlanner{proposals: sc.proposals},
			Tools:   stubTools{content: content, runID: sc.runID},
			Catalog: demoCatalog,
			Clock:   &stubClock{at: base.Add(time.Duration(i) * 7 * time.Minute)},
		})

		start := engine.Start{
			RunID:      sc.runID,
			Scope:      domain.Scope{Company: "acme", Area: sc.area},
			AgentID:    sc.agent,
			VersionID:  "v3",
			OnBehalfOf: "ana.souza",
			Pack:       gate.NewPack("crm.lookup", "kb.search", "crm.note", "crm.refund"),
			Budget:     budget,
			Trigger:    "cron",
		}

		// Turn until the run stops making progress: finished, awaiting a human
		// or parked. The step ceiling in the budget bounds the loop.
		for turn := 0; turn < 12; turn++ {
			st, err := runner.Advance(ctx, start)
			if err != nil {
				return fmt.Errorf("advance %s: %w", sc.runID, err)
			}
			if st.Done || st.Phase == engine.PhaseAwaitingApproval || st.Phase == engine.PhaseParked {
				break
			}
		}
	}
	return nil
}

var demoCatalog = staticCatalog{
	"crm.lookup": domain.EffectRead,
	"kb.search":  domain.EffectRead,
	"crm.note":   domain.EffectWrite,
	"crm.refund": domain.EffectFinancial,
}

type staticCatalog map[domain.ToolID]domain.Effect

func (s staticCatalog) Effect(id domain.ToolID) (domain.Effect, bool) {
	e, ok := s[id]
	return e, ok
}

type stubPlanner struct {
	proposals []engine.Proposal
	turn      int
}

func (p *stubPlanner) Plan(context.Context, engine.PlanInput) (engine.Proposal, error) {
	p.turn++
	if p.turn > len(p.proposals) {
		return engine.Proposal{Done: true, Outcome: "completed"}, nil
	}
	return p.proposals[p.turn-1], nil
}

// stubTools answers every call with a fixed body, put in the same content
// store the run's own steps reference.
//
// It used to mint a `memory://` reference and store nothing, which worked only
// while the demo runner had no content store to check it against. A reference
// that resolves to nothing is exactly what an erased one looks like, so a demo
// built on them would teach the console to render every finished run as erased.
type stubTools struct {
	content engine.ContentStore
	runID   domain.RunID
}

func (s stubTools) Invoke(ctx context.Context, call engine.Call) (engine.ToolResult, error) {
	ref, err := s.content.Put(ctx, s.runID, int64(len(call.Tool)),
		[]byte(`{"demo":"`+string(call.Tool)+`"}`))
	if err != nil {
		return engine.ToolResult{}, fmt.Errorf("demo: store result: %w", err)
	}
	return engine.ToolResult{
		ResultRef: ref,
		// Everything a tool returns from outside the platform is untrusted by
		// default; the label propagates from here through the run.
		Labels: domain.NewLabels(domain.LabelUntrusted),
		Cost:   domain.Cost{Micros: 2_400},
	}, nil
}

type stubClock struct {
	at time.Time
	n  int
}

func (c *stubClock) Now() time.Time {
	c.n++
	return c.at.Add(time.Duration(c.n) * 900 * time.Millisecond)
}

func cost(in, out, micros int64) domain.Cost {
	return domain.Cost{InputTokens: in, OutputTokens: out, Micros: micros}
}

func cons(micros int64) domain.Consumption {
	return domain.Consumption{Micros: micros, ToolCalls: 1}
}
