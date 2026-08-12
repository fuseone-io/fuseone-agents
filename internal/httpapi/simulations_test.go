package httpapi

import (
	"context"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/simulate"
)

// Simulating is how an agent earns its way out of Draft (FU-10), so the two
// things that matter here are that a set is accepted whole or not at all, and
// that reading a report never depends on the process that produced it.

type fakeSimulations struct {
	submitted []simulate.Job
	err       error
	report    simulate.Report
}

func (f *fakeSimulations) Submit(job simulate.Job) error {
	if f.err != nil {
		return f.err
	}
	f.submitted = append(f.submitted, job)
	return nil
}

func (f *fakeSimulations) Report(context.Context, string) (simulate.Report, error) {
	return f.report, nil
}

func simulatable(t *testing.T, sims *fakeSimulations) *Server {
	t.Helper()
	resolve := func(context.Context, domain.AgentID, domain.VersionID) (engine.Start, engine.Planner, error) {
		return engine.Start{}, stubPlanner{}, nil
	}
	return NewServer(ledger.NewMemory(), "test").WithAgents(triggerable(t)).
		WithSimulations(sims, resolve, engine.NewMemoryContent())
}

type stubPlanner struct{}

func (stubPlanner) Plan(context.Context, engine.PlanInput) (engine.Proposal, error) {
	return engine.Proposal{Done: true}, nil
}

func startSimulation(cases string) openapi.StartSimulationRequestObject {
	return openapi.StartSimulationRequestObject{
		AgentId: "triage",
		Params:  openapi.StartSimulationParams{IdempotencyKey: "sim-intent-0001"},
		Body:    &openapi.StartSimulationJSONRequestBody{Cases: cases},
	}
}

func TestStartSimulation_acceptsTheSetAndNamesTheSimulation(t *testing.T) {
	t.Parallel()

	sims := &fakeSimulations{}
	resp, err := simulatable(t, sims).StartSimulation(
		inArea("cx", domain.RoleAuthor),
		startSimulation(`{"assunto":"cobrança"}`+"\n"+`{"assunto":"acesso"}`+"\n"))
	if err != nil {
		t.Fatalf("StartSimulation: %v", err)
	}

	accepted, ok := resp.(openapi.StartSimulation202JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the set accepted", resp)
	}
	if accepted.Cases != 2 || accepted.Id == "" {
		t.Errorf("accepted = %+v", accepted)
	}
	if len(sims.submitted) != 1 || len(sims.submitted[0].Cases) != 2 {
		t.Fatalf("submitted = %+v", sims.submitted)
	}
	// The job carries the id it answered with, or the caller polls a report
	// that will never exist.
	if sims.submitted[0].ID != accepted.Id {
		t.Errorf("job %q, answered %q", sims.submitted[0].ID, accepted.Id)
	}
}

func TestStartSimulation_aLineThatIsNotJSON_refusesTheWholeFile(t *testing.T) {
	t.Parallel()

	sims := &fakeSimulations{}
	resp, err := simulatable(t, sims).StartSimulation(
		inArea("cx", domain.RoleAuthor),
		startSimulation(`{"assunto":"cobrança"}`+"\nnão é json\n"))
	if err != nil {
		t.Fatalf("StartSimulation: %v", err)
	}

	bad, ok := resp.(openapi.StartSimulation400ApplicationProblemPlusJSONResponse)
	if !ok {
		t.Fatalf("response = %T, want a refusal", resp)
	}
	// Named, so the author can fix the export rather than guess.
	if bad.Detail == nil || !strings.Contains(*bad.Detail, "2") {
		t.Errorf("detail = %v, want the line named", bad.Detail)
	}
	// Nothing started: half a set is coverage that lies.
	if len(sims.submitted) != 0 {
		t.Error("a set with a bad line was submitted anyway")
	}
}

func TestStartSimulation_whenTheQueueIsFull_saysSoRatherThanFailing(t *testing.T) {
	t.Parallel()

	sims := &fakeSimulations{err: simulate.ErrBusy}
	resp, err := simulatable(t, sims).StartSimulation(
		inArea("cx", domain.RoleAuthor), startSimulation(`{"n":1}`))
	if err != nil {
		t.Fatalf("StartSimulation: %v", err)
	}
	if _, ok := resp.(openapi.StartSimulation409ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want a conflict", resp)
	}
}

func TestStartSimulation_withoutTheAuthorityToPublish_isForbidden(t *testing.T) {
	t.Parallel()

	sims := &fakeSimulations{}
	// Simulating spends real money at a real provider and is the gate an
	// agent passes before it may be published. Reading runs is not that.
	resp, err := simulatable(t, sims).StartSimulation(
		inArea("cx", domain.RoleAuditor), startSimulation(`{"n":1}`))
	if err != nil {
		t.Fatalf("StartSimulation: %v", err)
	}
	if _, ok := resp.(openapi.StartSimulation403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want it refused", resp)
	}
	if len(sims.submitted) != 0 {
		t.Error("it was submitted anyway")
	}
}

func TestGetSimulation_rendersTheRowsAndWhatEachActWas(t *testing.T) {
	t.Parallel()

	sims := &fakeSimulations{report: simulate.Report{
		ID: "sim-1", Agent: "triage", Version: "v2", Expected: 2, Running: true,
		Cases: []simulate.Case{{
			RunID: "run-1", Settled: simulate.SettledParked, Steps: 4,
			Cost: domain.Cost{Micros: 1500, InputTokens: 900},
			Acted: []simulate.Act{{
				Step: "Responder", Tool: "crm.refund", Effect: domain.EffectFinancial,
				Verdict: domain.VerdictBlock, Rule: "capability",
			}},
		}},
	}}

	resp, err := simulatable(t, sims).GetSimulation(
		inArea("cx", domain.RoleAuthor),
		openapi.GetSimulationRequestObject{AgentId: "triage", SimulationId: "sim-1"})
	if err != nil {
		t.Fatalf("GetSimulation: %v", err)
	}

	got, ok := resp.(openapi.GetSimulation200JSONResponse)
	if !ok {
		t.Fatalf("response = %T", resp)
	}
	// Seven of twenty rather than seven: a report read while it is still
	// being written has to say so.
	if got.Expected != 2 || !got.Running || len(got.Cases) != 1 {
		t.Fatalf("report = %+v", got)
	}
	act := (*got.Cases[0].Acted)[0]
	if act.Verdict != openapi.VerdictBlock || act.Effect != openapi.Financial || act.Reached {
		t.Errorf("act = %+v", act)
	}
	// The rule, never only the verdict.
	if act.Rule == nil || *act.Rule != "capability" {
		t.Errorf("rule = %v", act.Rule)
	}
}

func TestGetSimulation_withoutTheAgent_isNotFound(t *testing.T) {
	t.Parallel()

	resp, err := simulatable(t, &fakeSimulations{}).GetSimulation(
		inArea("cx", domain.RoleAuthor),
		openapi.GetSimulationRequestObject{AgentId: "outro", SimulationId: "sim-1"})
	if err != nil {
		t.Fatalf("GetSimulation: %v", err)
	}
	if _, ok := resp.(openapi.GetSimulation404ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want not found", resp)
	}
}
