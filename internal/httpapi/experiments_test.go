package httpapi

import (
	gocontext "context"
	"encoding/json"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// Starting an experiment pins a dataset version, an agent version and a configuration,
// and opens each case once per repetition (NT-015 § Experiments). These are the doors in
// front of that: who may, what may be asked for, and what an answer counts.

func experimentServer(t *testing.T, store *ledger.Memory) *Server {
	t.Helper()
	server, _ := experimentFixture(t, store, servedAgent())
	return server
}

// servedAgent is the agent an experiment runs: one version, on a pair this installation
// serves.
func servedAgent() *fakeDetail {
	return &fakeDetail{versions: []domain.AgentSummary{servedVersion("v2")}}
}

func servedVersion(id domain.VersionID) domain.AgentSummary {
	return domain.AgentSummary{
		ID: "triage", VersionID: id, Name: "Atendimento", Latest: true,
		Scope:    domain.Scope{Company: "acme", Area: "cx"},
		Provider: "anthropic", Model: "test-model",
	}
}

// serving is which providers this installation configures, and which of them it could ask.
type serving struct{ configured, usable map[string]bool }

func (s serving) Configured(name string) bool { return s.configured[name] || s.usable[name] }
func (s serving) Usable(name string) bool     { return s.usable[name] }

func experimentFixture(t *testing.T, store *ledger.Memory, agents Agents) (*Server, *fakeCorpus) {
	t.Helper()
	content := engine.NewMemoryContent()
	corpusCase := func(id string) domain.RegressionCase {
		ref, err := content.Put(gocontext.Background(), domain.RunID("origin-"+id), 1, []byte(`{"case":"`+id+`"}`))
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		return domain.RegressionCase{ID: id, Agent: "triage", InputRef: ref}
	}
	at2 := []domain.RegressionCase{corpusCase("a"), corpusCase("b")}
	corpus := &fakeCorpus{listed: at2, latest: 2,
		versions: map[int64][]domain.RegressionCase{1: {corpusCase("a")}, 2: at2}}
	return NewServer(store, "test").WithAgents(agents).
		WithContent(content).WithCases(content).
		WithRegressions(corpus).
		WithProviderNames(serving{
			configured: map[string]bool{"switched-off": true},
			usable:     map[string]bool{"anthropic": true, "openai": true},
		}), corpus
}

func startExperiment(body openapi.ExperimentRequest) openapi.StartExperimentRequestObject {
	return openapi.StartExperimentRequestObject{
		AgentId: "triage",
		Params:  openapi.StartExperimentParams{IdempotencyKey: "exp-intent-0001"},
		Body:    &body,
	}
}

func TestStartExperiment_opensEachCaseOncePerRepetitionUnderItsConfiguration(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	config := openapi.ExperimentConfig{Model: ptr("other-model"), Effort: ptr("low")}

	resp, err := experimentServer(t, store).StartExperiment(inArea("cx", domain.RoleAuthor),
		startExperiment(openapi.ExperimentRequest{Config: &config, Repetitions: ptr(2)}))
	if err != nil {
		t.Fatalf("StartExperiment: %v", err)
	}
	accepted, ok := resp.(openapi.StartExperiment202JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want it accepted", resp)
	}
	if accepted.Cases != 2 || accepted.Repetitions != 2 || accepted.Runs != 4 || accepted.DatasetVersion != 2 {
		t.Fatalf("accepted = %+v, want two cases of version 2 twice", accepted)
	}
	ids, err := store.SimulationRuns(gocontext.Background(), accepted.Id)
	if err != nil || len(ids) != 4 {
		t.Fatalf("runs = %v err=%v, want four", ids, err)
	}
	for _, id := range ids {
		steps, _ := store.Read(gocontext.Background(), id, domain.FirstSeq)
		var started domain.RunStartedPayload
		if err := json.Unmarshal(steps[0].Payload, &started); err != nil {
			t.Fatalf("payload: %v", err)
		}
		want := domain.ExperimentConfig{Model: "other-model", Effort: "low"}
		if started.Experiment != accepted.Id || started.ExperimentConfig == nil ||
			*started.ExperimentConfig != want || started.DatasetVersion != 2 {
			t.Errorf("run opened as %+v, want an experiment run under %+v", started, want)
		}
	}
}

func TestStartExperiment_aDatasetVersion_isTheOneReplayed(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()

	resp, err := experimentServer(t, store).StartExperiment(inArea("cx", domain.RoleAuthor),
		startExperiment(openapi.ExperimentRequest{DatasetVersion: ptr(int64(1))}))
	if err != nil {
		t.Fatalf("StartExperiment: %v", err)
	}
	accepted, ok := resp.(openapi.StartExperiment202JSONResponse)
	if !ok || accepted.Cases != 1 || accepted.DatasetVersion != 1 {
		t.Fatalf("response = %+v, want the one case of version 1", resp)
	}
}

func TestStartExperiment_askedAgain_reachesTheSameExperiment(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	server := experimentServer(t, store)

	var ids []string
	for range 2 {
		resp, err := server.StartExperiment(inArea("cx", domain.RoleAuthor),
			startExperiment(openapi.ExperimentRequest{}))
		if err != nil {
			t.Fatalf("StartExperiment: %v", err)
		}
		ids = append(ids, resp.(openapi.StartExperiment202JSONResponse).Id)
	}
	runs, _ := store.SimulationRuns(gocontext.Background(), ids[0])
	if ids[0] != ids[1] || len(runs) != 2 {
		t.Errorf("ids = %v with %d runs, want one experiment of two runs", ids, len(runs))
	}
}

func TestStartExperiment_whatCannotBeRun_isRefusedAndOpensNothing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body openapi.ExperimentRequest
	}{
		{"a provider nobody configured", openapi.ExperimentRequest{
			Config: &openapi.ExperimentConfig{Provider: ptr("elsewhere"), Model: ptr("x")}}},
		{"only a provider, nobody configured", openapi.ExperimentRequest{
			Config: &openapi.ExperimentConfig{Provider: ptr("elsewhere")}}},
		{"no repetitions", openapi.ExperimentRequest{Repetitions: ptr(0)}},
		{"six repetitions", openapi.ExperimentRequest{Repetitions: ptr(6)}},
		{"a dataset version that does not exist", openapi.ExperimentRequest{DatasetVersion: ptr(int64(9))}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			store := ledger.NewMemory()

			resp, err := experimentServer(t, store).StartExperiment(inArea("cx", domain.RoleAuthor),
				startExperiment(c.body))
			if err != nil {
				t.Fatalf("StartExperiment: %v", err)
			}
			if _, ok := resp.(openapi.StartExperiment400ApplicationProblemPlusJSONResponse); !ok {
				t.Fatalf("response = %T, want it refused", resp)
			}
			if runs, _ := store.Runs(gocontext.Background()); len(runs) != 0 {
				t.Error("runs were opened anyway")
			}
		})
	}
}

func TestStartExperiment_withoutTheAuthorityToPublish_isForbidden(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()

	resp, err := experimentServer(t, store).StartExperiment(inArea("cx", domain.RoleAuditor),
		startExperiment(openapi.ExperimentRequest{}))
	if err != nil {
		t.Fatalf("StartExperiment: %v", err)
	}
	if _, ok := resp.(openapi.StartExperiment403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want it refused", resp)
	}
}

// Every field is optional and an absent one is the version's own (NT-015): a provider alone
// runs the published model under it, and is recorded as asked.
func TestStartExperiment_onlyAProvider_isAcceptedAndRecordedAsAsked(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()

	resp, err := experimentServer(t, store).StartExperiment(inArea("cx", domain.RoleAuthor),
		startExperiment(openapi.ExperimentRequest{Config: &openapi.ExperimentConfig{Provider: ptr("openai")}}))
	if err != nil {
		t.Fatalf("StartExperiment: %v", err)
	}
	accepted, ok := resp.(openapi.StartExperiment202JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want it accepted", resp)
	}
	ids, _ := store.SimulationRuns(gocontext.Background(), accepted.Id)
	steps, _ := store.Read(gocontext.Background(), ids[0], domain.FirstSeq)
	var started domain.RunStartedPayload
	if err := json.Unmarshal(steps[0].Payload, &started); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if started.ExperimentConfig == nil || *started.ExperimentConfig != (domain.ExperimentConfig{Provider: "openai"}) {
		t.Errorf("configuration = %+v, want the provider alone", started.ExperimentConfig)
	}
}
