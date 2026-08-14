package httpapi

import (
	gocontext "context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
Starting an agent whose corrections stopped holding.

The gate is on starting rather than on publishing, and the reason is not
taste: a version's identifier is the digest of its own bytes, so it does not
exist until it is published and nothing can have been simulated against it.
Publishing writes a definition down. Starting is what makes it act.
*/

func TestSetAgentPaused_corpusBrokeAgainstThisVersion_startIsRefused(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	simulateCase(t, store, "run-1", "sim-9", "caso-estorno")

	resp, err := gated(t, store, brokenCorpus("caso-estorno")).SetAgentPaused(
		inArea("cx", domain.RoleAuthor), startAgent(false))
	if err != nil {
		t.Fatalf("SetAgentPaused: %v", err)
	}

	refused, ok := resp.(openapi.SetAgentPaused409ApplicationProblemPlusJSONResponse)
	if !ok {
		t.Fatalf("response = %T, want it refused", resp)
	}
	// Named, not counted: the whole reason a correction carries an identifier
	// is so somebody can be sent to the one that broke.
	if refused.Detail == nil || !strings.Contains(*refused.Detail, "caso-estorno") {
		t.Errorf("detail = %v, want the case named", refused.Detail)
	}
}

func TestSetAgentPaused_nothingHasBeenSimulated_startIsAllowed(t *testing.T) {
	t.Parallel()

	// FU-10 already stands between an agent and its first publication. Making
	// every later correction to an instruction demand a fresh battery would
	// turn the corpus into something people route around, and a corpus that
	// says nothing about this version says nothing.
	pub := newPublisher()
	resp, err := gatedWith(t, ledger.NewMemory(), brokenCorpus("caso-estorno"), pub).
		SetAgentPaused(inArea("cx", domain.RoleAuthor), startAgent(false))
	if err != nil {
		t.Fatalf("SetAgentPaused: %v", err)
	}
	if _, ok := resp.(openapi.SetAgentPaused204Response); !ok {
		t.Fatalf("response = %T, want it started", resp)
	}
	if pub.paused["triage"] {
		t.Error("the agent was left stopped")
	}
}

func TestSetAgentPaused_corpusHolds_startIsAllowed(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	simulateCase(t, store, "run-1", "sim-9", "caso-estorno")

	held := []domain.RegressionCase{{
		ID: "caso-estorno", Agent: "triage",
		Expectations: []domain.Expectation{{Kind: domain.ExpectNeverCalls, Value: "crm.refund"}},
	}}
	resp, err := gated(t, store, held).SetAgentPaused(
		inArea("cx", domain.RoleAuthor), startAgent(false))
	if err != nil {
		t.Fatalf("SetAgentPaused: %v", err)
	}
	if _, ok := resp.(openapi.SetAgentPaused204Response); !ok {
		t.Fatalf("response = %T, want it started", resp)
	}
}

func TestSetAgentPaused_stoppingIsNeverRefused(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	simulateCase(t, store, "run-1", "sim-9", "caso-estorno")

	// Whatever else is true, somebody must always be able to stop an agent.
	// A gate that can refuse that is a gate that keeps a broken agent running.
	pub := newPublisher()
	pub.paused["triage"] = false

	resp, err := gatedWith(t, store, brokenCorpus("caso-estorno"), pub).
		SetAgentPaused(inArea("cx", domain.RoleAuthor), startAgent(true))
	if err != nil {
		t.Fatalf("SetAgentPaused: %v", err)
	}
	if _, ok := resp.(openapi.SetAgentPaused204Response); !ok {
		t.Fatalf("response = %T, want it stopped", resp)
	}
	if !pub.paused["triage"] {
		t.Error("the agent is still running")
	}
}

// A battery run against the version before this one says nothing about this
// one. Reading it as though it did is how a green report certifies bytes that
// were never simulated.
func TestSetAgentPaused_batteryRanAgainstAnotherVersion_startIsAllowed(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	simulateVersion(t, store, "run-1", "sim-old", "caso-estorno", "v1")

	resp, err := gated(t, store, brokenCorpus("caso-estorno")).SetAgentPaused(
		inArea("cx", domain.RoleAuthor), startAgent(false))
	if err != nil {
		t.Fatalf("SetAgentPaused: %v", err)
	}
	if _, ok := resp.(openapi.SetAgentPaused204Response); !ok {
		t.Fatalf("response = %T, want it started", resp)
	}
}

// --- fixtures ---------------------------------------------------------------

func startAgent(paused bool) openapi.SetAgentPausedRequestObject {
	return openapi.SetAgentPausedRequestObject{
		AgentId: "triage",
		Body:    &openapi.SetAgentPausedJSONRequestBody{Paused: paused},
	}
}

func gated(t *testing.T, store *ledger.Memory, corpus []domain.RegressionCase) *Server {
	t.Helper()
	return gatedWith(t, store, corpus, newPublisher())
}

func gatedWith(
	t *testing.T, store *ledger.Memory, corpus []domain.RegressionCase, pub *publisher,
) *Server {
	t.Helper()
	return NewServer(store, "test").WithAgents(triggerable(t)).WithPublisher(pub).
		WithRegressions(&fakeCorpus{listed: corpus}).WithBatteries(store)
}

func brokenCorpus(id string) []domain.RegressionCase {
	return []domain.RegressionCase{{
		ID: id, Agent: "triage",
		// The run below never reaches this tool, so the correction is unmet.
		Expectations: []domain.Expectation{{Kind: domain.ExpectCalls, Value: "crm.lookup"}},
	}}
}

func simulateCase(t *testing.T, store *ledger.Memory, run, simulation, kase string) {
	t.Helper()
	simulateVersion(t, store, run, simulation, kase, "v2")
}

// simulateVersion writes a settled simulated run, which is what a battery is.
func simulateVersion(
	t *testing.T, store *ledger.Memory, run, simulation, kase, version string,
) {
	t.Helper()
	ctx := gocontext.Background()
	started, _ := json.Marshal(domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: simulation, Case: kase,
	})
	step := domain.Step{
		RunID: domain.RunID(run), Kind: domain.StepRunStarted,
		Scope:   domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", VersionID: domain.VersionID(version), Payload: started,
	}
	if _, err := store.Append(ctx, step); err != nil {
		t.Fatalf("open the simulated run: %v", err)
	}

	settled, _ := json.Marshal(domain.RunFinishedPayload{Outcome: "answered"})
	step.Kind, step.Payload = domain.StepRunFinished, settled
	if _, err := store.Append(ctx, step); err != nil {
		t.Fatalf("settle the simulated run: %v", err)
	}
}
