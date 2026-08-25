package httpapi

import (
	gocontext "context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

func TestGetAgentTrust_aRegressionBlocksPromotion(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	batteryFor(t, store, "v1", "sim-1", "estorno", withTool("crm.lookup"))
	batteryFor(t, store, "v2", "sim-2", "estorno")

	trust := readTrust(t, trustable(t, store), "triage", "")
	if trust.Status != openapi.AgentTrustStatusNeedsReview {
		t.Fatalf("status = %q, want needs_review", trust.Status)
	}
	assertEvidence(t, trust, openapi.AgentTrustEvidenceIdVersion,
		openapi.AgentTrustEvidenceStatusBad, openapi.VersionRegressed)
	assertEvidence(t, trust, openapi.AgentTrustEvidenceIdSimulation,
		openapi.AgentTrustEvidenceStatusBad, openapi.SimulationRegressed)
}

func TestGetAgentTrust_withoutSimulationCorpusAsksForEvidence(t *testing.T) {
	t.Parallel()

	trust := readTrust(t, trustableWithCorpus(t, ledger.NewMemory(), nil), "triage", "")
	if trust.Status != openapi.AgentTrustStatusNeedsEvidence {
		t.Fatalf("status = %q, want needs_evidence", trust.Status)
	}
	assertEvidence(t, trust, openapi.AgentTrustEvidenceIdSimulation,
		openapi.AgentTrustEvidenceStatusMissing, openapi.SimulationMissingCorpus)
}

func TestGetAgentTrust_aPendingApprovalIsGovernanceNotFailure(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	appendTo(t, store, "run-waiting", "v2", domain.StepRunStarted,
		domain.RunStartedPayload{Trigger: "manual"})
	appendTo(t, store, "run-waiting", "v2", domain.StepApprovalRequested,
		domain.ApprovalRequestedPayload{Tool: "crm.reply", Rule: "taint"})

	trust := readTrust(t, trustable(t, store), "triage", "")
	if trust.Status != openapi.AgentTrustStatusNeedsEvidence {
		t.Fatalf("status = %q, want needs_evidence", trust.Status)
	}
	assertEvidence(t, trust, openapi.AgentTrustEvidenceIdRuns,
		openapi.AgentTrustEvidenceStatusGood, openapi.RunsWaiting)
	assertEvidence(t, trust, openapi.AgentTrustEvidenceIdDecisions,
		openapi.AgentTrustEvidenceStatusUnknown, openapi.DecisionsWaiting)
}

func TestGetAgentTrust_costRegressionIsNamedByVersion(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	finishedRun(t, store, "run-v1", "v1", 100_000)
	finishedRun(t, store, "run-v2", "v2", 900_000)

	trust := readTrust(t, trustable(t, store), "triage", "")
	assertEvidence(t, trust, openapi.AgentTrustEvidenceIdCost,
		openapi.AgentTrustEvidenceStatusBad, openapi.CostIncreased)
}

func TestGetAgentTrust_newToolGrantIsAReviewSignal(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	server := trustableWithVersions(t, store, &fakeDetail{versions: []domain.AgentSummary{
		trustVersion("v2", "crm.lookup", "crm.reply"),
		trustVersion("v1", "crm.lookup"),
	}})

	trust := readTrust(t, server, "triage", "")
	assertEvidence(t, trust, openapi.AgentTrustEvidenceIdCapabilities,
		openapi.AgentTrustEvidenceStatusBad, openapi.CapabilitiesAdded)
}

func TestGetAgentTrust_gateBlocksAreNamedByVersion(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	finishedRun(t, store, "run-v1", "v1", 10_000)
	appendTo(t, store, "run-v2", "v2", domain.StepRunStarted,
		domain.RunStartedPayload{Trigger: "manual"})
	appendTo(t, store, "run-v2", "v2", domain.StepGateDecided,
		domain.GateDecidedPayload{
			Tool: "crm.reply", Verdict: domain.VerdictBlock, Rule: "policy",
		})
	appendTo(t, store, "run-v2", "v2", domain.StepRunFinished,
		domain.RunFinishedPayload{Outcome: "blocked"})

	trust := readTrust(t, trustable(t, store), "triage", "")
	assertEvidence(t, trust, openapi.AgentTrustEvidenceIdPolicy,
		openapi.AgentTrustEvidenceStatusBad, openapi.PolicyBlocksIncreased)
}

func TestGetAgentTrust_oldRunEvidenceFallsOutsideTheTrustWindow(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	old := time.Now().Add(-trustEvidenceWindow - time.Hour)
	appendAt(t, store, "run-old", "v2", domain.StepRunStarted,
		domain.RunStartedPayload{Trigger: "manual"}, old)
	appendAt(t, store, "run-old", "v2", domain.StepGateDecided,
		domain.GateDecidedPayload{
			Tool: "crm.reply", Verdict: domain.VerdictBlock, Rule: "policy",
		}, old.Add(time.Second))
	appendAt(t, store, "run-old", "v2", domain.StepRunFinished,
		domain.RunFinishedPayload{Outcome: "blocked"}, old.Add(2*time.Second))
	finishedRun(t, store, "run-recent", "v2", 10_000)

	trust := readTrust(t, trustable(t, store), "triage", "")
	if trust.Window.From.IsZero() || !trust.Window.Until.After(trust.Window.From) {
		t.Fatalf("trust window = %+v, want a concrete interval", trust.Window)
	}
	assertEvidence(t, trust, openapi.AgentTrustEvidenceIdPolicy,
		openapi.AgentTrustEvidenceStatusGood, openapi.PolicyNoBlocks)
}

func TestGetAgentTrust_inAnotherAreaReadsAsAbsent(t *testing.T) {
	t.Parallel()

	resp, err := trustable(t, ledger.NewMemory()).GetAgentTrust(
		inArea("billing", domain.RoleAuthor),
		openapi.GetAgentTrustRequestObject{AgentId: "triage"})
	if err != nil {
		t.Fatalf("GetAgentTrust: %v", err)
	}
	if _, absent := resp.(openapi.GetAgentTrust404ApplicationProblemPlusJSONResponse); !absent {
		t.Fatalf("response = %T, want 404", resp)
	}
}

func readTrust(t *testing.T, server *Server, agent, version string) openapi.AgentTrust {
	t.Helper()
	req := openapi.GetAgentTrustRequestObject{AgentId: agent}
	if version != "" {
		req.Params.Version = ptr(version)
	}
	resp, err := server.GetAgentTrust(inArea("cx", domain.RoleAuthor), req)
	if err != nil {
		t.Fatalf("GetAgentTrust: %v", err)
	}
	got, ok := resp.(openapi.GetAgentTrust200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want trust", resp)
	}
	return openapi.AgentTrust(got)
}

func assertEvidence(
	t *testing.T, trust openapi.AgentTrust, id openapi.AgentTrustEvidenceId,
	status openapi.AgentTrustEvidenceStatus, code openapi.AgentTrustEvidenceCode,
) {
	t.Helper()
	for _, item := range trust.Evidence {
		if item.Id != id {
			continue
		}
		if item.Status != status || item.Code != code {
			t.Fatalf("%s = %s/%s, want %s/%s",
				id, item.Status, item.Code, status, code)
		}
		return
	}
	t.Fatalf("%s evidence missing in %+v", id, trust.Evidence)
}

func trustable(t *testing.T, store *ledger.Memory) *Server {
	t.Helper()
	return trustableWithCorpus(t, store, []domain.RegressionCase{{
		ID: "estorno", Agent: "triage",
		Expectations: []domain.Expectation{{Kind: domain.ExpectCalls, Value: "crm.lookup"}},
	}})
}

func trustableWithCorpus(
	t *testing.T, store *ledger.Memory, corpus []domain.RegressionCase,
) *Server {
	t.Helper()
	return trustableWithVersions(t, store, twoVersions(t)).
		WithRegressions(&fakeCorpus{listed: corpus}).WithBatteries(store)
}

func trustableWithVersions(
	t *testing.T, store *ledger.Memory, agents *fakeDetail,
) *Server {
	t.Helper()
	stages := &fakeStages{stage: domain.StageCopilot}
	return NewServer(store, "test").WithAgents(agents).
		WithPauses(runningAgent{}).WithPromotions(stages)
}

func trustVersion(version domain.VersionID, tools ...domain.ToolID) domain.AgentSummary {
	return domain.AgentSummary{
		ID: "triage", VersionID: version, Name: "Atendimento",
		Scope: domain.Scope{Company: "acme", Area: "cx"},
		Tools: tools,
	}
}

func appendAt(
	t *testing.T, store *ledger.Memory, run domain.RunID,
	version string, kind domain.StepKind, payload any, at time.Time,
) {
	t.Helper()
	if _, err := store.Append(gocontext.Background(), domain.Step{
		RunID: run, Kind: kind, Scope: domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", VersionID: domain.VersionID(version),
		At: at.UTC(), Payload: mustPayload(t, payload),
	}); err != nil {
		t.Fatalf("append %s: %v", kind, err)
	}
}

func finishedRun(
	t *testing.T, store *ledger.Memory, run domain.RunID,
	version domain.VersionID, micros int64,
) {
	t.Helper()
	appendTo(t, store, run, string(version), domain.StepRunStarted,
		domain.RunStartedPayload{Trigger: "manual"})
	cost := mustPayload(t, domain.PlannedPayload{})
	if _, err := store.Append(gocontext.Background(), domain.Step{
		RunID: run, Kind: domain.StepPlanned,
		Scope:   domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", VersionID: version,
		At: time.Now().UTC(), Payload: cost, Cost: domain.Cost{Micros: micros},
	}); err != nil {
		t.Fatalf("append planned: %v", err)
	}
	appendTo(t, store, run, string(version), domain.StepRunFinished,
		domain.RunFinishedPayload{Outcome: "answered"})
}
