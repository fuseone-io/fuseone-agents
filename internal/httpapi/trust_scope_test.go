package httpapi

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
A version is read only where it was published.

An agent republished from finance to cx keeps its finance versions, and their batteries.
Somebody who may read cx is allowed the agent as it is now, and has no claim on a version
only ever published in finance — named, or picked as the one before. Every door answers
404, as for a version that does not exist.
*/

// republishedInCx is an agent whose v1 was published in finance and whose v2 is in cx, with
// a battery run against each.
func republishedInCx(t *testing.T, store *ledger.Memory) *fakeDetail {
	t.Helper()
	batteryFor(t, store, "v1", "sim-1", "estorno", withTool("crm.lookup"))
	batteryFor(t, store, "v2", "sim-2", "estorno")
	agents := twoVersions(t)
	agents.versions[1].Scope = domain.Scope{Company: "acme", Area: "finance"}
	return agents
}

// Trust already refuses a named version it may not read, and drops a previous one it may
// not read. These keep it that way.
func TestGetAgentTrust_aVersionPublishedInAnotherScope_isNotFound(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	server := trustable(t, store).WithAgents(republishedInCx(t, store))

	resp, err := server.GetAgentTrust(inArea("cx", domain.RoleAuthor),
		openapi.GetAgentTrustRequestObject{AgentId: "triage", Params: openapi.GetAgentTrustParams{Version: ptr("v1")}})
	if err != nil {
		t.Fatalf("GetAgentTrust: %v", err)
	}
	if _, ok := resp.(openapi.GetAgentTrust404ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("response = %T, want the finance version hidden from cx", resp)
	}
}

func TestGetAgentTrust_aPreviousVersionPublishedInAnotherScope_isLeftOut(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	server := trustable(t, store).WithAgents(republishedInCx(t, store))

	got := readTrust(t, server, "triage", "")

	if got.VersionId != "v2" || got.PreviousVersionId != nil {
		t.Errorf("trust = %s against %v, want v2 with no previous version named", got.VersionId, got.PreviousVersionId)
	}
}
