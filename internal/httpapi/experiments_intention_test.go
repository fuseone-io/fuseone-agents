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
An experiment is one intention, and its key names it (NT-015 § Experiments).

A retry reaches the runs the first attempt opened. It must not add runs of another
intention to them: an experiment holding two dataset versions, two agent versions or two
configurations compares nothing with anything. So a retry runs what the first attempt
recorded, a field it leaves out included, and one that states something else is refused.
*/

// startedRuns is every run an experiment opened, by how it was opened.
func startedRuns(t *testing.T, store *ledger.Memory, id string) []domain.Step {
	t.Helper()
	ids, err := store.SimulationRuns(gocontext.Background(), id)
	if err != nil {
		t.Fatalf("SimulationRuns: %v", err)
	}
	out := make([]domain.Step, 0, len(ids))
	for _, run := range ids {
		steps, err := store.Read(gocontext.Background(), run, domain.FirstSeq)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		out = append(out, steps[0])
	}
	return out
}

func startedPayload(t *testing.T, step domain.Step) domain.RunStartedPayload {
	t.Helper()
	var started domain.RunStartedPayload
	if err := json.Unmarshal(step.Payload, &started); err != nil {
		t.Fatalf("payload: %v", err)
	}
	return started
}

func accept(t *testing.T, server *Server, body openapi.ExperimentRequest) openapi.StartExperiment202JSONResponse {
	t.Helper()
	resp, err := server.StartExperiment(inArea("cx", domain.RoleAuthor), startExperiment(body))
	if err != nil {
		t.Fatalf("StartExperiment: %v", err)
	}
	accepted, ok := resp.(openapi.StartExperiment202JSONResponse)
	if !ok {
		t.Fatalf("response = %T %+v, want it accepted", resp, resp)
	}
	return accepted
}

func TestStartExperiment_askedAgainStatingAnotherIntention_isRefusedAndOpensNothingMore(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		first, second openapi.ExperimentRequest
	}{
		{"another dataset version",
			openapi.ExperimentRequest{DatasetVersion: ptr(int64(1))},
			openapi.ExperimentRequest{DatasetVersion: ptr(int64(2))}},
		{"more repetitions",
			openapi.ExperimentRequest{Repetitions: ptr(1)},
			openapi.ExperimentRequest{Repetitions: ptr(2)}},
		{"another configuration",
			openapi.ExperimentRequest{Config: &openapi.ExperimentConfig{Effort: ptr("low")}},
			openapi.ExperimentRequest{Config: &openapi.ExperimentConfig{Effort: ptr("high")}}},
		{"the version as published, after an override",
			openapi.ExperimentRequest{Config: &openapi.ExperimentConfig{Effort: ptr("low")}},
			openapi.ExperimentRequest{Config: &openapi.ExperimentConfig{}}},
		{"another agent version",
			openapi.ExperimentRequest{},
			openapi.ExperimentRequest{Version: ptr("v1")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			store := ledger.NewMemory()
			agents := &fakeDetail{versions: []domain.AgentSummary{servedVersion("v2"), servedVersion("v1")}}
			server, _ := experimentFixture(t, store, agents)
			first := accept(t, server, c.first)

			resp, err := server.StartExperiment(inArea("cx", domain.RoleAuthor), startExperiment(c.second))
			if err != nil {
				t.Fatalf("StartExperiment: %v", err)
			}
			if _, ok := resp.(openapi.StartExperiment409ApplicationProblemPlusJSONResponse); !ok {
				t.Fatalf("response = %T %+v, want the divergence refused", resp, resp)
			}
			if runs := startedRuns(t, store, first.Id); len(runs) != first.Runs {
				t.Errorf("%d runs after the retry, want the %d the first attempt opened", len(runs), first.Runs)
			}
		})
	}
}

// A field the retry leaves out is the first attempt's, not today's: the dataset moved on,
// a version was published, and the retry still runs, and reports, what was asked for.
func TestStartExperiment_askedAgainLeavingFieldsOut_runsWhatTheFirstAttemptRecorded(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	agents := &fakeDetail{versions: []domain.AgentSummary{servedVersion("v2")}}
	server, corpus := experimentFixture(t, store, agents)
	corpus.latest, corpus.listed = 1, corpus.versions[1]
	first := accept(t, server, openapi.ExperimentRequest{
		Repetitions: ptr(2), Config: &openapi.ExperimentConfig{Effort: ptr("low")},
	})

	corpus.latest, corpus.listed = 2, corpus.versions[2]
	agents.versions = []domain.AgentSummary{servedVersion("v3"), servedVersion("v2")}
	again := accept(t, server, openapi.ExperimentRequest{})

	if again.Id != first.Id || again.DatasetVersion != 1 || again.Cases != 1 || again.Repetitions != 2 {
		t.Errorf("retry = %+v, want the first attempt %+v", again, first)
	}
	runs := startedRuns(t, store, first.Id)
	if len(runs) != 2 {
		t.Fatalf("%d runs, want the one case twice", len(runs))
	}
	for _, step := range runs {
		started := startedPayload(t, step)
		if step.VersionID != "v2" || started.DatasetVersion != 1 || started.ExperimentConfig == nil ||
			started.ExperimentConfig.Effort != "low" {
			t.Errorf("run opened on %s as %+v, want v2, dataset 1, effort low", step.VersionID, started)
		}
	}
}

// publishingDuring publishes v3 once the handler has read which version is published, so
// any run that resolves "the version published now" on its own reaches v3.
type publishingDuring struct {
	*fakeDetail
	read int
}

func (p *publishingDuring) Versions(ctx gocontext.Context, agent domain.AgentID) ([]domain.AgentSummary, error) {
	p.read++
	if p.read > 1 {
		p.versions = []domain.AgentSummary{servedVersion("v3"), servedVersion("v2")}
	}
	return p.fakeDetail.Versions(ctx, agent)
}

func TestStartExperiment_aVersionPublishedWhileItOpens_reachesNoneOfItsRuns(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	server, _ := experimentFixture(t, store, &publishingDuring{fakeDetail: servedAgent()})

	accepted := accept(t, server, openapi.ExperimentRequest{Repetitions: ptr(2)})

	runs := startedRuns(t, store, accepted.Id)
	if len(runs) != 4 {
		t.Fatalf("%d runs, want two cases twice", len(runs))
	}
	for _, step := range runs {
		if step.VersionID != "v2" {
			t.Errorf("a run opened on %s, want every run on v2, published when it was asked", step.VersionID)
		}
	}
}

// The pair checked is the one the runs would plan on: the configuration over the version,
// field by field, asked of the providers this installation could actually call.
func TestStartExperiment_anEffectivePairNobodyCanServe_isRefusedAndOpensNothing(t *testing.T) {
	t.Parallel()
	removed := servedVersion("v2")
	removed.Provider = "removed"
	cases := []struct {
		name    string
		version domain.AgentSummary
		config  *openapi.ExperimentConfig
	}{
		{"only a model, on a published provider since removed", removed,
			&openapi.ExperimentConfig{Model: ptr("other-model")}},
		{"only an effort, on a published provider since removed", removed,
			&openapi.ExperimentConfig{Effort: ptr("low")}},
		{"a provider switched off", servedVersion("v2"),
			&openapi.ExperimentConfig{Provider: ptr("switched-off")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			store := ledger.NewMemory()
			server, _ := experimentFixture(t, store, &fakeDetail{versions: []domain.AgentSummary{c.version}})

			resp, err := server.StartExperiment(inArea("cx", domain.RoleAuthor),
				startExperiment(openapi.ExperimentRequest{Config: c.config}))
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

// Two first attempts under one key, with different bodies, must not interleave: each
// reads what is recorded and opens inside a fence no other experiment of the agent shares.
func TestStartExperiment_opensInsideTheExclusiveFence(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	server, corpus := experimentFixture(t, store, servedAgent())

	accept(t, server, openapi.ExperimentRequest{})

	if corpus.exclusive != 1 || corpus.held != 0 {
		t.Errorf("exclusive=%d shared=%d, want the experiment opened under the exclusive fence alone",
			corpus.exclusive, corpus.held)
	}
}

/*
A retry that states nothing else reaches the experiment its key names, whatever moved
since it was accepted (the Idempotency-Key contract). The runs exist, and so does their
cost, so answering 400 tells a caller nothing happened when it did. What changed decides
only whether the retry can still add the runs the first attempt left unopened.
*/
func TestStartExperiment_askedAgainAfterTheWorldMoved_reachesTheRunsItOpened(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		moved func(*Server, *fakeCorpus)
	}{
		{"its provider removed", func(s *Server, _ *fakeCorpus) {
			s.providerNames.(serving).usable["anthropic"] = false
		}},
		{"one of its cases erased", func(_ *Server, corpus *fakeCorpus) {
			corpus.versions[2][1].Erased = true
		}},
		{"its corpus erased", func(_ *Server, corpus *fakeCorpus) {
			for i := range corpus.versions[2] {
				corpus.versions[2][i].Erased = true
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			store := ledger.NewMemory()
			server, corpus := experimentFixture(t, store, servedAgent())
			body := openapi.ExperimentRequest{Repetitions: ptr(2)}
			first := accept(t, server, body)

			c.moved(server, corpus)
			again := accept(t, server, body)

			if again != first {
				t.Errorf("retry = %+v, want the answer the first attempt got, %+v", again, first)
			}
			if runs := startedRuns(t, store, first.Id); len(runs) != first.Runs {
				t.Errorf("%d runs after the retry, want the %d already opened", len(runs), first.Runs)
			}
		})
	}
}

// A first attempt cut short — its caller timed out and the request was cancelled — leaves
// cases unopened, and the retry is what opens them, under the intention already recorded.
func TestStartExperiment_askedAgainAfterAnInterruptedFirstAttempt_opensWhatIsMissing(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	server, _ := experimentFixture(t, store, servedAgent())
	id := experimentID("triage", "exp-intent-0001")
	occurrences, err := server.corpusOccurrences(gocontext.Background(), "triage", 2)
	if err != nil {
		t.Fatalf("corpus: %v", err)
	}
	if _, err := simulate.Open(gocontext.Background(), server.opener(), simulate.Batch{
		ID: id, Agent: "triage", By: "usr_ana", DatasetVersion: 2, Cases: occurrences.Cases[:1],
		Experiment: &simulate.Experiment{Version: "v2", Repetitions: 1},
	}); err != nil {
		t.Fatalf("the interrupted attempt: %v", err)
	}

	again := accept(t, server, openapi.ExperimentRequest{})

	if again.Id != id || again.Runs != 2 || again.Cases != 2 {
		t.Errorf("retry = %+v, want both cases of %s opened", again, id)
	}
}
