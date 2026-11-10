package httpapi

import (
	gocontext "context"
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
