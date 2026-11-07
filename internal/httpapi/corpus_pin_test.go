package httpapi

import (
	gocontext "context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
A battery is read against what it ran.

Each of these has a working set that disagrees with the version, because that is the
case the pin exists for: the corpus moved after the battery, and its answer must not.
*/

// simulatePinned writes a settled simulated run naming a version of the corpus.
func simulatePinned(
	t *testing.T, store *ledger.Memory, run, simulation, kase string, version int64,
) {
	t.Helper()
	appendPinned(t, store, "v2", run, simulation, kase, version)
}

func appendPinned(
	t *testing.T, store *ledger.Memory, agentVersion, run, simulation, kase string, version int64,
) {
	t.Helper()
	base := time.Now().Add(-30 * time.Minute)
	appendAt(t, store, domain.RunID(run), agentVersion, domain.StepRunStarted, domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: simulation, Case: kase,
		DatasetVersion: version,
	}, base)
	appendAt(t, store, domain.RunID(run), agentVersion, domain.StepRunFinished,
		domain.RunFinishedPayload{Outcome: "answered"}, base.Add(time.Second))
}

func heldCase(id string) domain.RegressionCase {
	return domain.RegressionCase{
		ID: id, Agent: "triage",
		Expectations: []domain.Expectation{{Kind: domain.ExpectNeverCalls, Value: "crm.refund"}},
	}
}

func pinnedServer(
	store *ledger.Memory, t *testing.T, working []domain.RegressionCase, at1 []domain.RegressionCase,
) *Server {
	t.Helper()
	return NewServer(store, "test").WithAgents(triggerable(t)).WithPublisher(newPublisher()).
		WithRegressions(&fakeCorpus{
			listed: working, latest: 2, versions: map[int64][]domain.RegressionCase{1: at1},
		}).WithBatteries(store)
}

// Added after the battery, so it never ran. Not checked is not broken, and it no longer
// stops a start by itself (NT-015).
func TestSetAgentPaused_aCaseAddedAfterTheBattery_doesNotRefuseTheStart(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	simulatePinned(t, store, "run-1", "sim-9", "caso-velho", 1)

	working := append([]domain.RegressionCase{heldCase("caso-velho")}, brokenCorpus("caso-novo")...)
	resp, err := pinnedServer(store, t, working, []domain.RegressionCase{heldCase("caso-velho")}).
		SetAgentPaused(inArea("cx", domain.RoleAuthor), startAgent(false))
	if err != nil {
		t.Fatalf("SetAgentPaused: %v", err)
	}
	if _, ok := resp.(openapi.SetAgentPaused204Response); !ok {
		t.Fatalf("response = %T, want it started", resp)
	}
}

// Deleting the case from the working set does not undo what this version did on it.
func TestSetAgentPaused_aCaseThatBrokeAndWasDeletedSince_stillRefusesTheStart(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	simulatePinned(t, store, "run-1", "sim-9", "caso-estorno", 1)

	resp, err := pinnedServer(store, t, nil, brokenCorpus("caso-estorno")).
		SetAgentPaused(inArea("cx", domain.RoleAuthor), startAgent(false))
	if err != nil {
		t.Fatalf("SetAgentPaused: %v", err)
	}
	if _, ok := resp.(openapi.SetAgentPaused409ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want it refused", resp)
	}
}

// A battery from before versions has no pin, and is read as it always was.
func TestSetAgentPaused_anUnpinnedBattery_isReadAgainstTheCorpusAsItIs(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	simulateCase(t, store, "run-1", "sim-9", "caso-estorno")

	resp, err := gated(t, store, brokenCorpus("caso-estorno")).SetAgentPaused(
		inArea("cx", domain.RoleAuthor), startAgent(false))
	if err != nil {
		t.Fatalf("SetAgentPaused: %v", err)
	}
	if _, ok := resp.(openapi.SetAgentPaused409ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want it refused", resp)
	}
}

// Grown since, and the report of a past battery does not gain a case it never ran.
func TestGetSimulation_readsABatteryAgainstItsVersion(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	simulatePinned(t, store, "run-1", "sim-9", "caso-velho", 1)
	working := append([]domain.RegressionCase{heldCase("caso-velho")}, brokenCorpus("caso-novo")...)

	resp, err := pinnedServer(store, t, working, []domain.RegressionCase{heldCase("caso-velho")}).GetSimulation(
		inArea("cx", domain.RoleAuthor),
		openapi.GetSimulationRequestObject{AgentId: "triage", SimulationId: "sim-9"})
	if err != nil {
		t.Fatalf("GetSimulation: %v", err)
	}
	got, ok := resp.(openapi.GetSimulation200JSONResponse)
	if !ok {
		t.Fatalf("response = %T", resp)
	}
	if got.Held == nil || *got.Held != 1 || got.Broken == nil || *got.Broken != 0 || len(got.Cases) != 1 {
		t.Errorf("held=%v broken=%v cases=%d, want the one case it ran, held", got.Held, got.Broken, len(got.Cases))
	}
}

// Added between the batteries and holding on the newer: nobody fixed anything.
func TestCompareVersions_aCaseAddedBetweenTheBatteries_isNotFixed(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	appendPinned(t, store, "v1", "run-1", "sim-1", "estorno", 1)
	appendPinned(t, store, "v2", "run-2", "sim-2", "estorno", 2)
	appendPinned(t, store, "v2", "run-3", "sim-2", "novo", 2)
	server := NewServer(store, "test").WithAgents(twoVersions(t)).WithBatteries(store).
		WithRegressions(&fakeCorpus{
			// What the working set says by now is not what either battery ran.
			listed: brokenCorpus("estorno"),
			versions: map[int64][]domain.RegressionCase{
				1: {heldCase("estorno")},
				2: {heldCase("estorno"), heldCase("novo")},
			},
		})

	resp, err := server.CompareVersions(inArea("cx", domain.RoleAuthor),
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
	if got.Fixed != 0 || got.Regressed != 0 {
		t.Errorf("fixed = %d, regressed = %d, want neither", got.Fixed, got.Regressed)
	}
	for _, row := range got.Cases {
		if row.Id == "estorno" && (row.Was != openapi.Held || row.Now != openapi.Held) {
			t.Errorf("estorno = %+v, want it held on both sides, as each version says", row)
		}
		if row.Id == "novo" && row.Was != openapi.NotComparable {
			t.Errorf("novo = %+v, want it not comparable", row)
		}
	}
}

// The trust center reads the same battery, so it has to read it the same way: a case
// added since is not coverage this version lost.
func TestGetAgentTrust_readsABatteryAgainstItsVersion(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	simulatePinned(t, store, "run-1", "sim-9", "caso-velho", 1)
	working := append([]domain.RegressionCase{heldCase("caso-velho")}, brokenCorpus("caso-novo")...)

	trust := readTrust(t, pinnedServer(store, t, working, []domain.RegressionCase{heldCase("caso-velho")}), "triage", "")

	assertEvidence(t, trust, openapi.AgentTrustEvidenceIdSimulation,
		openapi.AgentTrustEvidenceStatusGood, openapi.SimulationReady)
}

func TestStartSimulation_theCorpus_namesTheVersionItRan(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	content := engine.NewMemoryContent()
	kept, err := content.PutFor(gocontext.Background(), "regression", "triage", 1, []byte(`{"a":1}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	corpus := &fakeCorpus{latest: 5, listed: []domain.RegressionCase{{ID: "reg-kept", InputRef: kept}}}
	server := NewServer(store, "test").WithAgents(triggerable(t)).
		WithContent(content).WithCases(content).WithRegressions(corpus)

	resp, err := server.StartSimulation(inArea("cx", domain.RoleAuthor), corpusSimulation())
	if err != nil {
		t.Fatalf("StartSimulation: %v", err)
	}
	accepted, ok := resp.(openapi.StartSimulation202JSONResponse)
	if !ok {
		t.Fatalf("response = %T", resp)
	}
	if got := pinOfBattery(t, store, accepted.Id); got != 5 {
		t.Errorf("dataset version = %d, want 5", got)
	}
}

// An uploaded set is not a version of anything.
func TestStartSimulation_anUploadedSet_namesNoVersion(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()

	resp, err := simulatable(t, store).StartSimulation(inArea("cx", domain.RoleAuthor), startSimulation(`{"n":1}`))
	if err != nil {
		t.Fatalf("StartSimulation: %v", err)
	}
	if got := pinOfBattery(t, store, resp.(openapi.StartSimulation202JSONResponse).Id); got != 0 {
		t.Errorf("dataset version = %d, want none", got)
	}
}

// The input is there and what it expected is gone. Running it spends money on a run
// nothing can judge.
func TestStartSimulation_aCaseWhoseExpectationsAreGone_doesNotRun(t *testing.T) {
	t.Parallel()
	content := engine.NewMemoryContent()
	bg := gocontext.Background()
	live, err := content.PutFor(bg, "regression", "triage", 1, []byte(`{"a":1}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	corpus := &fakeCorpus{listed: []domain.RegressionCase{
		{ID: "runs", InputRef: live}, {ID: "gone", InputRef: live, Erased: true},
	}}
	server := NewServer(ledger.NewMemory(), "test").WithAgents(triggerable(t)).
		WithContent(content).WithCases(content).WithRegressions(corpus)

	resp, err := server.StartSimulation(inArea("cx", domain.RoleAuthor), corpusSimulation())
	if err != nil {
		t.Fatalf("StartSimulation: %v", err)
	}
	if got := resp.(openapi.StartSimulation202JSONResponse).Cases; got != 1 {
		t.Errorf("cases = %d, want the one that can be judged", got)
	}
}

func pinOfBattery(t *testing.T, store *ledger.Memory, simulation string) int64 {
	t.Helper()
	ctx := gocontext.Background()
	runs, err := store.SimulationRuns(ctx, simulation)
	if err != nil || len(runs) == 0 {
		t.Fatalf("runs = %v, err = %v", runs, err)
	}
	steps, err := store.Read(ctx, runs[0], domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var started domain.RunStartedPayload
	if err := json.Unmarshal(steps[0].Payload, &started); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return started.DatasetVersion
}
