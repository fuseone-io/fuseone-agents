package httpapi

import (
	gocontext "context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
Is the new version better than the old one?

Nothing is stored for this. Each side is the newest battery run against that
version, folded from the ledger, and the diff is a fold of two folds.
*/

func TestCompareVersions_aCorrectionThatStoppedHolding_isTheFirstRow(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	// v1 held the correction; v2 never reaches the tool.
	batteryFor(t, store, "v1", "sim-1", "estorno", withTool("crm.lookup"))
	batteryFor(t, store, "v2", "sim-2", "estorno")

	resp, err := comparable(t, store).CompareVersions(
		inArea("cx", domain.RoleAuthor),
		openapi.CompareVersionsRequestObject{
			AgentId: "triage",
			Params:  openapi.CompareVersionsParams{From: ptr("v1"), To: ptr("v2")},
		})
	if err != nil {
		t.Fatalf("CompareVersions: %v", err)
	}

	got, ok := resp.(openapi.CompareVersions200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the comparison", resp)
	}
	if got.Regressed != 1 || got.Fixed != 0 {
		t.Fatalf("regressed=%d fixed=%d, want one regression", got.Regressed, got.Fixed)
	}
	if len(got.Cases) != 1 || got.Cases[0].Id != "estorno" {
		t.Fatalf("cases = %+v", got.Cases)
	}
	if got.Cases[0].Was != openapi.Held || got.Cases[0].Now != openapi.Broke {
		t.Errorf("row = %+v, want held then broken", got.Cases[0])
	}
}

// Answering with an empty diff would read as "nothing changed" about two
// versions that were never compared at all.
func TestCompareVersions_aVersionNobodyRanTheCorpusAgainst_isAConflict(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	batteryFor(t, store, "v1", "sim-1", "estorno")

	resp, err := comparable(t, store).CompareVersions(
		inArea("cx", domain.RoleAuthor),
		openapi.CompareVersionsRequestObject{
			AgentId: "triage",
			Params:  openapi.CompareVersionsParams{From: ptr("v1"), To: ptr("v9")},
		})
	if err != nil {
		t.Fatalf("CompareVersions: %v", err)
	}
	if _, ok := resp.(openapi.CompareVersions409ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want a conflict", resp)
	}
}

// Reading an agent is not a permission an auditor lacks, but this reads what
// a version cost — and the answer is the same one every other agent read
// gives, refused for somebody who cannot see the area at all.
func TestCompareVersions_inAnotherArea_readsAsAbsent(t *testing.T) {
	t.Parallel()

	resp, err := comparable(t, ledger.NewMemory()).CompareVersions(
		inArea("financeiro", domain.RoleAuthor),
		openapi.CompareVersionsRequestObject{AgentId: "triage"})
	if err != nil {
		t.Fatalf("CompareVersions: %v", err)
	}
	if _, ok := resp.(openapi.CompareVersions404ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want not found", resp)
	}
}

// --- fixtures ---------------------------------------------------------------

func comparable(t *testing.T, store *ledger.Memory) *Server {
	t.Helper()
	corpus := &fakeCorpus{listed: []domain.RegressionCase{{
		ID: "estorno", Agent: "triage",
		Expectations: []domain.Expectation{{Kind: domain.ExpectCalls, Value: "crm.lookup"}},
	}}}
	return NewServer(store, "test").WithAgents(twoVersions(t)).
		WithRegressions(corpus).WithBatteries(store)
}

func twoVersions(t *testing.T) *fakeDetail {
	t.Helper()
	return &fakeDetail{versions: []domain.AgentSummary{
		{
			ID: "triage", VersionID: "v2", Name: "Atendimento", Latest: true,
			Scope: domain.Scope{Company: "acme", Area: "cx"},
		},
		{
			ID: "triage", VersionID: "v1", Name: "Atendimento",
			Scope: domain.Scope{Company: "acme", Area: "cx"},
		},
	}}
}

type reached func(t *testing.T, store *ledger.Memory, run domain.RunID, version string)

func withTool(tool domain.ToolID) reached {
	return func(t *testing.T, store *ledger.Memory, run domain.RunID, version string) {
		t.Helper()
		appendTo(t, store, run, version, domain.StepGateDecided, domain.GateDecidedPayload{
			Tool: tool, Effect: domain.EffectRead, Verdict: domain.VerdictAllow,
		})
		appendTo(t, store, run, version, domain.StepToolCalled, domain.ToolCalledPayload{Tool: tool})
	}
}

// batteryFor writes one settled simulated run of one case against one version.
func batteryFor(
	t *testing.T, store *ledger.Memory, version, simulation, kase string, acts ...reached,
) {
	t.Helper()
	run := domain.RunID(simulation + "-" + kase)
	appendTo(t, store, run, version, domain.StepRunStarted, domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: simulation, Case: kase,
	})
	for _, act := range acts {
		act(t, store, run, version)
	}
	appendTo(t, store, run, version, domain.StepRunFinished,
		domain.RunFinishedPayload{Outcome: "answered"})
}

func appendTo(
	t *testing.T, store *ledger.Memory, run domain.RunID,
	version string, kind domain.StepKind, payload any,
) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	if _, err = store.Append(gocontext.Background(), domain.Step{
		RunID: run, Kind: kind, Scope: domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", VersionID: domain.VersionID(version),
		At: time.Now().UTC(), Payload: raw,
	}); err != nil {
		t.Fatalf("append %s: %v", kind, err)
	}
}
