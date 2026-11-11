package httpapi

import (
	gocontext "context"
	"encoding/json"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/simulate"
)

/*
A simulation is read only under the agent and the scope its runs were recorded under.

The id is whatever a caller typed. Read under the route's agent and today's scope alone, it
folded another agent's simulation, and the runs an agent recorded under finance before it
was republished under cx, for anybody who may read cx. Each answers 404, as an id that names
nothing does.
*/

func simulationScoped(v domain.AgentSummary, area string) domain.AgentSummary {
	v.Scope = domain.Scope{Company: "acme", Area: domain.AreaID(area)}
	return v
}

func openSimulation(t *testing.T, server *Server, id string, agent domain.AgentID) {
	t.Helper()
	if _, err := simulate.Open(gocontext.Background(), server.opener(), simulate.Batch{
		ID: id, Agent: agent, Cases: []simulate.Occurrence{{Input: []byte(`{}`)}},
	}); err != nil {
		t.Fatalf("open %s: %v", id, err)
	}
}

func getSimulation(t *testing.T, server *Server, id string) openapi.GetSimulationResponseObject {
	t.Helper()
	resp, err := server.GetSimulation(inArea("cx", domain.RoleAuditor),
		openapi.GetSimulationRequestObject{AgentId: "triage", SimulationId: id})
	if err != nil {
		t.Fatalf("GetSimulation(%s): %v", id, err)
	}
	return resp
}

func TestGetSimulation_anotherAgentsOrOneThatDoesNotExist_isNotFound(t *testing.T) {
	t.Parallel()
	other := simulationScoped(servedVersion("v1"), "cx")
	other.ID = "other"
	server, _ := experimentFixture(t, ledger.NewMemory(),
		&fakeDetail{versions: []domain.AgentSummary{servedVersion("v2"), other}})
	openSimulation(t, server, "sim_other_1", "other")

	for _, id := range []string{"sim_other_1", "sim_triage_never"} {
		if _, ok := getSimulation(t, server, id).(openapi.GetSimulation404ApplicationProblemPlusJSONResponse); !ok {
			t.Errorf("GetSimulation(%s) was answered, want it not found", id)
		}
	}
}

func TestGetSimulation_recordedBeforeTheAgentWasRepublishedElsewhere_isNotFound(t *testing.T) {
	t.Parallel()
	agents := &fakeDetail{versions: []domain.AgentSummary{simulationScoped(servedVersion("v1"), "finance")}}
	server, _ := experimentFixture(t, ledger.NewMemory(), agents)
	openSimulation(t, server, "sim_triage_finance", "triage")
	agents.versions = []domain.AgentSummary{servedVersion("v2"), simulationScoped(servedVersion("v1"), "finance")}
	openSimulation(t, server, "sim_triage_cx", "triage")

	if _, ok := getSimulation(t, server, "sim_triage_finance").(openapi.GetSimulation404ApplicationProblemPlusJSONResponse); !ok {
		t.Error("the finance simulation was answered to cx, want it not found")
	}
	if _, ok := getSimulation(t, server, "sim_triage_cx").(openapi.GetSimulation200JSONResponse); !ok {
		t.Error("the cx simulation was not answered, want it read")
	}
}

// openRun records one simulated run's opening under a simulation id, for a reader to find.
func openRun(t *testing.T, store *ledger.Memory, run domain.RunID, started domain.RunStartedPayload) {
	t.Helper()
	payload, err := json.Marshal(started)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := store.Append(gocontext.Background(), domain.Step{
		RunID: run, Kind: domain.StepRunStarted, Scope: domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", VersionID: "v2", OnBehalfOf: "ana", IdemKey: string(run), Payload: payload,
	}); err != nil {
		t.Fatalf("append %s: %v", run, err)
	}
}

/*
A judgment's id is not a simulation to read (NT-016). Its runs are this agent's, recorded
where the caller may read, so every other door lets them through. Folded as a simulation,
a judge's answer would be shown as an agent's.

The anchor is the same run without the judge: it is read, so the 404 is the judge's.
*/
func TestGetSimulation_aJudgmentsID_isNotFound(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	server, _ := experimentFixture(t, store, &fakeDetail{versions: []domain.AgentSummary{servedVersion("v2")}})
	opening := func(simulation string) domain.RunStartedPayload {
		return domain.RunStartedPayload{
			Trigger: "simulation", Simulated: true, Simulation: simulation,
			Case: "a", DatasetVersion: 2, Experiment: "exp_triage_1",
		}
	}
	judged := opening("jdg_triage_1")
	judged.Judge = &domain.JudgeOpening{
		Run: "run-exp-1", RubricDigest: "sha256:rubric", Provider: "anthropic", Model: "judge",
		CeilingMicros: 1000, Repetitions: 1, Repetition: 1,
	}
	openRun(t, store, "run-judge-1", judged)
	openRun(t, store, "run-anchor-1", opening("jdg_triage_anchor"))

	if _, ok := getSimulation(t, server, "jdg_triage_1").(openapi.GetSimulation404ApplicationProblemPlusJSONResponse); !ok {
		t.Error("a judgment was read as a simulation, want it not found")
	}
	if _, ok := getSimulation(t, server, "jdg_triage_anchor").(openapi.GetSimulation200JSONResponse); !ok {
		t.Error("the same run without a judge was not read; the 404 above proves nothing")
	}
}
