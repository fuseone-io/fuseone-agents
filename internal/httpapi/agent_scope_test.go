package httpapi

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
A version is read only where it was published, and the history lists only those.

An agent republished from one area to another keeps its old versions where they were. A
reader of where it is now is allowed the agent as it is, and has no claim on a version only
ever published elsewhere: not its definition, and not that it exists. Named, it is a 404, as
a version that does not exist is; in the history, it is absent. Checked moving each way.
*/

// movedAgent is an agent whose v1 was published in one area and whose v2 is in another.
func movedAgent(from, to string) *fakeDetail {
	at := func(id domain.VersionID, area string, latest bool) domain.AgentSummary {
		return domain.AgentSummary{
			ID: "triage", VersionID: id, Name: "Atendimento", Latest: latest,
			Scope: domain.Scope{Company: "acme", Area: domain.AreaID(area)},
		}
	}
	return &fakeDetail{
		versions:     []domain.AgentSummary{at("v2", to, true), at("v1", from, false)},
		instructions: map[domain.VersionID]string{"v1": "a definição antiga", "v2": "a atual"},
	}
}

func getAgentAs(t *testing.T, agents *fakeDetail, area, version string) openapi.GetAgentResponseObject {
	t.Helper()
	req := openapi.GetAgentRequestObject{AgentId: "triage"}
	if version != "" {
		req.Params.Version = ptr(version)
	}
	resp, err := NewServer(ledger.NewMemory(), "test").WithAgents(agents).
		GetAgent(inArea(area, domain.RoleAuthor), req)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	return resp
}

func TestGetAgent_aVersionPublishedInAnotherArea_isNotFound(t *testing.T) {
	t.Parallel()
	for _, move := range [][2]string{{"finance", "cx"}, {"cx", "finance"}} {
		agents := movedAgent(move[0], move[1])

		resp := getAgentAs(t, agents, move[1], "v1")

		if _, ok := resp.(openapi.GetAgent404ApplicationProblemPlusJSONResponse); !ok {
			t.Errorf("moved %s → %s: v1 read as %T by %s, want it not found", move[0], move[1], resp, move[1])
		}
	}
}

func TestGetAgent_theHistory_listsOnlyVersionsTheReaderMayRead(t *testing.T) {
	t.Parallel()
	for _, move := range [][2]string{{"finance", "cx"}, {"cx", "finance"}} {
		resp := getAgentAs(t, movedAgent(move[0], move[1]), move[1], "")

		got, ok := resp.(openapi.GetAgent200JSONResponse)
		if !ok {
			t.Fatalf("moved %s → %s: read as %T, want the current version", move[0], move[1], resp)
		}
		if len(got.Versions) != 1 || got.Versions[0].VersionId != "v2" {
			t.Errorf("moved %s → %s: history %+v, want only v2", move[0], move[1], got.Versions)
		}
	}
}

// Where it was, with nothing readable where it is now, the agent itself is absent: there
// is no current version this reader may be shown, and the old one is not offered instead.
func TestGetAgent_movedAwayFromTheReader_isNotFoundAtAnyVersion(t *testing.T) {
	t.Parallel()
	agents := movedAgent("cx", "finance")
	for _, version := range []string{"", "v1"} {
		if _, ok := getAgentAs(t, agents, "cx", version).(openapi.GetAgent404ApplicationProblemPlusJSONResponse); !ok {
			t.Errorf("version %q read by cx after the agent moved to finance, want it not found", version)
		}
	}
}
