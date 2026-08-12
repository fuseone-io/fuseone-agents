package httpapi

import (
	gocontext "context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// Simulating is how an agent earns its way out of Draft (FU-10). The two
// things that matter here are that a set is accepted whole or not at all, and
// that what comes back is a fold of the runs rather than a record kept beside
// them.

func simulatable(t *testing.T, store *ledger.Memory) *Server {
	t.Helper()
	content := engine.NewMemoryContent()
	return NewServer(store, "test").WithAgents(triggerable(t)).
		WithContent(content).WithCases(content)
}

func startSimulation(cases string) openapi.StartSimulationRequestObject {
	return openapi.StartSimulationRequestObject{
		AgentId: "triage",
		Params:  openapi.StartSimulationParams{IdempotencyKey: "sim-intent-0001"},
		Body:    &openapi.StartSimulationJSONRequestBody{Cases: cases},
	}
}

func TestStartSimulation_opensOneRunPerCase_eachNamingTheSimulation(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	resp, err := simulatable(t, store).StartSimulation(
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
		t.Fatalf("accepted = %+v", accepted)
	}

	// The runs are the queue, so they exist the moment this answers — and
	// every one of them says which simulation it belongs to.
	ids, err := store.SimulationRuns(gocontext.Background(), accepted.Id)
	if err != nil {
		t.Fatalf("SimulationRuns: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("runs = %v, want one per case", ids)
	}
	steps, _ := store.Read(gocontext.Background(), ids[0], domain.FirstSeq)
	var p domain.RunStartedPayload
	if err := json.Unmarshal(steps[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	// Marked, so no worker holding real tools can ever claim it.
	if !p.Simulated || p.Simulation != accepted.Id {
		t.Errorf("run opened as %+v", p)
	}
}

func TestStartSimulation_aLineThatIsNotJSON_refusesTheWholeFile(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	resp, err := simulatable(t, store).StartSimulation(
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
	// And nothing opened: half a set is coverage that lies.
	runs, _ := store.Runs(gocontext.Background())
	if len(runs) != 0 {
		t.Errorf("runs = %v, want none", runs)
	}
}

func TestStartSimulation_withoutTheAuthorityToPublish_isForbidden(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	// Simulating spends real money at a real provider and is the gate an
	// agent passes before it may be published. Reading runs is not that.
	resp, err := simulatable(t, store).StartSimulation(
		inArea("cx", domain.RoleAuditor), startSimulation(`{"n":1}`))
	if err != nil {
		t.Fatalf("StartSimulation: %v", err)
	}
	if _, ok := resp.(openapi.StartSimulation403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want it refused", resp)
	}
	if runs, _ := store.Runs(gocontext.Background()); len(runs) != 0 {
		t.Error("runs were opened anyway")
	}
}

func TestStartSimulation_whenNotOneCaseCanOpen_saysSoRatherThanAccepting(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	server := simulatable(t, store).WithPauses(pausedAgent{})

	resp, err := server.StartSimulation(inArea("cx", domain.RoleAuthor), startSimulation(`{"n":1}`))
	if err != nil {
		t.Fatalf("StartSimulation: %v", err)
	}
	// Answering as though it had started would leave the author polling a
	// report that will never have a row.
	if _, ok := resp.(openapi.StartSimulation409ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want a conflict", resp)
	}
}

type pausedAgent struct{}

func (pausedAgent) IsPaused(gocontext.Context, domain.AgentID) (bool, error) { return true, nil }

func TestGetSimulation_foldsTheRunsIntoRowsAndNamesTheRuleThatStopped(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	server := simulatable(t, store)
	ctx := inArea("cx", domain.RoleAuthor)

	started, err := server.StartSimulation(ctx, startSimulation(`{"n":1}`))
	if err != nil {
		t.Fatalf("StartSimulation: %v", err)
	}
	id := started.(openapi.StartSimulation202JSONResponse).Id

	ids, _ := store.SimulationRuns(gocontext.Background(), id)
	appendStep(t, store, ids[0], domain.StepPlanned, domain.PlannedPayload{Node: "Responder"})
	appendStep(t, store, ids[0], domain.StepGateDecided, domain.GateDecidedPayload{
		Tool: "crm.refund", Effect: domain.EffectFinancial,
		Verdict: domain.VerdictBlock, Rule: "capability",
	})
	appendStep(t, store, ids[0], domain.StepParked, domain.ParkedPayload{Reason: "blocked"})

	resp, err := server.GetSimulation(ctx, openapi.GetSimulationRequestObject{
		AgentId: "triage", SimulationId: id,
	})
	if err != nil {
		t.Fatalf("GetSimulation: %v", err)
	}
	got, ok := resp.(openapi.GetSimulation200JSONResponse)
	if !ok {
		t.Fatalf("response = %T", resp)
	}

	if len(got.Cases) != 1 || got.Running {
		t.Fatalf("report = %+v", got)
	}
	act := (*got.Cases[0].Acted)[0]
	if act.Verdict != openapi.VerdictBlock || act.Effect != openapi.Financial || act.Reached {
		t.Errorf("act = %+v", act)
	}
	// The rule, never only the verdict: "blocked by policy" tells an author
	// nothing about what to change.
	if act.Rule == nil || *act.Rule != "capability" {
		t.Errorf("rule = %v", act.Rule)
	}
	if act.Step == nil || *act.Step != "Responder" {
		t.Errorf("step = %v, want the one it was proposed in", act.Step)
	}
}

func TestGetSimulation_withoutTheAgent_isNotFound(t *testing.T) {
	t.Parallel()

	resp, err := simulatable(t, ledger.NewMemory()).GetSimulation(
		inArea("cx", domain.RoleAuthor),
		openapi.GetSimulationRequestObject{AgentId: "outro", SimulationId: "sim-1"})
	if err != nil {
		t.Fatalf("GetSimulation: %v", err)
	}
	if _, ok := resp.(openapi.GetSimulation404ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want not found", resp)
	}
}

func appendStep(t *testing.T, store *ledger.Memory, id domain.RunID, kind domain.StepKind, payload any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	if _, err := store.Append(gocontext.Background(), domain.Step{
		RunID: id, Kind: kind, Scope: domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", VersionID: "v2", Payload: raw,
	}); err != nil {
		t.Fatalf("append %s: %v", kind, err)
	}
}
