package httpapi

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// An agent's experiments, for the console to pick two to compare (NT-015 § Experiments).

func listExperiments(t *testing.T, server *Server, area string) []openapi.ExperimentListing {
	t.Helper()
	resp, err := server.ListExperiments(inArea(area, domain.RoleAuditor),
		openapi.ListExperimentsRequestObject{AgentId: "triage"})
	if err != nil {
		t.Fatalf("ListExperiments: %v", err)
	}
	listed, ok := resp.(openapi.ListExperiments200JSONResponse)
	if !ok {
		t.Fatalf("response = %T %+v, want the list", resp, resp)
	}
	return listed.Experiments
}

func TestListExperiments_saysWhatEachPinned(t *testing.T) {
	t.Parallel()
	server, _ := experimentFixture(t, ledger.NewMemory(), servedAgent())
	id := acceptKeyed(t, server, "exp-intent-0001", openapi.ExperimentRequest{
		Repetitions: ptr(2), DatasetVersion: ptr(int64(1)), Config: &openapi.ExperimentConfig{Effort: ptr("low")},
	})

	got := listExperiments(t, server, "cx")

	if len(got) != 1 {
		t.Fatalf("listed %+v, want the one experiment", got)
	}
	e := got[0]
	if e.Id != id || e.DatasetVersion != 1 || e.Version != "v2" || deref(e.Config.Effort) != "low" ||
		e.Repetitions != 2 || e.Runs != 2 || e.Cases != 1 || e.OpenedAt.IsZero() {
		t.Errorf("listing = %+v, want the one case of dataset 1, twice, on v2 at effort low", e)
	}
}

// An agent republished elsewhere keeps its old experiments where they were recorded, and a
// reader of where it is now is not shown them.
func TestListExperiments_leavesOutOneRecordedInAnotherScope(t *testing.T) {
	t.Parallel()
	server, _, _, finance := movedToCx(t)
	cx := acceptKeyed(t, server, "exp-intent-0002", openapi.ExperimentRequest{})

	got := listExperiments(t, server, "cx")

	if len(got) != 1 || got[0].Id != cx {
		t.Errorf("listed %+v, want only %s and not %s", got, cx, finance)
	}
}
