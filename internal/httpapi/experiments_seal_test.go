package httpapi

import (
	gocontext "context"
	"sync/atomic"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/trigger"
)

/*
Two calls with one key, where the second decided nothing before the fence (NT-018 phase 2,
release 2b).

B, a retry leaving the seal out, reaches the route before any intention exists and waits at
the fence. A runs on seal A and opens the one case whose input is readable. The other
case's input comes back and seal A is erased. When B enters, what it runs on must be read
inside the fence: the intention A recorded, on a seal that is now unreadable, so B answers
from A's run and opens nothing on the erased seal.
*/

const testSealA = "0b7c3e52-8a43-4b55-9a3f-0d6a3c5f9e11"

// fakeCandidates is one seal of the served agent.
type fakeCandidates struct{ erased atomic.Bool }

func (f *fakeCandidates) List(gocontext.Context, domain.AgentID, []domain.Scope) ([]spec.CandidateSeal, error) {
	return nil, nil
}

func (f *fakeCandidates) Read(gocontext.Context, domain.AgentID, domain.VersionID, []domain.Scope) (spec.Candidate, error) {
	return spec.Candidate{}, spec.ErrCandidateNotFound
}

func (f *fakeCandidates) Seal(gocontext.Context, domain.AgentID, string, []domain.Scope) (spec.CandidateSeal, error) {
	return spec.CandidateSeal{}, spec.ErrCandidateNotFound
}

func (f *fakeCandidates) Candidate(_ gocontext.Context, agent domain.AgentID, seal string) (trigger.Candidate, bool, error) {
	if seal != testSealA || agent != "triage" {
		return trigger.Candidate{}, false, nil
	}
	return trigger.Candidate{Agent: agent, Version: "vseal0000001", Scope: domain.Scope{Company: "acme", Area: "cx"}}, true, nil
}

func (f *fakeCandidates) Sealed(_ gocontext.Context, agent domain.AgentID, seal string) (spec.Spec, error) {
	if _, found, _ := f.Candidate(gocontext.Background(), agent, seal); !found {
		return spec.Spec{}, spec.ErrCandidateNotFound
	}
	if f.erased.Load() {
		return spec.Spec{}, domain.ErrCandidateUnreadable
	}
	return spec.Spec{ID: agent, Version: "vseal0000001", Provider: "anthropic"}, nil
}

// gatedCorpus holds the first call that reaches the fence until released, and lets every
// later one straight through.
type gatedCorpus struct {
	*fakeCorpus
	gated            atomic.Bool
	entered, release chan struct{}
}

func (g *gatedCorpus) Exclusive(
	ctx gocontext.Context, agents []domain.AgentID, inside func(gocontext.Context) error,
) error {
	if g.gated.CompareAndSwap(false, true) {
		close(g.entered)
		<-g.release
	}
	return g.fakeCorpus.Exclusive(ctx, agents, inside)
}

func TestStartExperiment_aSecondCallWithTheKey_resolvesItsTargetInsideTheFence(t *testing.T) {
	store := ledger.NewMemory()
	content := engine.NewMemoryContent()
	input := func(id string) string {
		ref, err := content.Put(gocontext.Background(), domain.RunID("origin-"+id), 1, []byte(`{"case":"`+id+`"}`))
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		return ref
	}
	cases := []domain.RegressionCase{
		{ID: "a", Agent: "triage", InputRef: input("a")}, {ID: "b", Agent: "triage", InputRef: input("b")},
	}
	if _, err := content.Erase(gocontext.Background(), "run", "origin-b", "test"); err != nil {
		t.Fatalf("erase b's input: %v", err)
	}
	corpus := &gatedCorpus{
		fakeCorpus: &fakeCorpus{listed: cases, latest: 1, versions: map[int64][]domain.RegressionCase{1: cases}},
		entered:    make(chan struct{}), release: make(chan struct{}),
	}
	candidates := &fakeCandidates{}
	server := NewServer(store, "test").WithAgents(servedAgent()).
		WithContent(content).WithCases(content).WithRegressions(corpus).WithCandidates(candidates).
		WithProviderNames(serving{usable: map[string]bool{"anthropic": true}})
	ctx := inArea("cx", domain.RoleAuthor)
	request := func(body openapi.ExperimentRequest) openapi.StartExperimentRequestObject {
		return openapi.StartExperimentRequestObject{AgentId: "triage",
			Params: openapi.StartExperimentParams{IdempotencyKey: "exp-race-0001"}, Body: &body}
	}

	second := make(chan openapi.StartExperimentResponseObject, 1)
	go func() {
		resp, err := server.StartExperiment(ctx, request(openapi.ExperimentRequest{}))
		if err != nil {
			t.Errorf("second call: %v", err)
		}
		second <- resp
	}()
	<-corpus.entered

	seal := testSealA
	resp, err := server.StartExperiment(ctx, request(openapi.ExperimentRequest{SealId: &seal}))
	first, ok := resp.(openapi.StartExperiment202JSONResponse)
	if err != nil || !ok || first.Runs != 1 {
		t.Fatalf("the first call = %T %+v (%v), want the one readable case opened on the seal", resp, resp, err)
	}
	if _, err := content.RestoreFor(gocontext.Background(), "run", "origin-b", 1, []byte(`{"case":"b"}`)); err != nil {
		t.Fatalf("restore b's input: %v", err)
	}
	candidates.erased.Store(true)
	close(corpus.release)

	got, ok := (<-second).(openapi.StartExperiment202JSONResponse)
	if !ok || got.Id != first.Id || got.Runs != 1 {
		t.Errorf("the second call = %+v, want the first call's one run and nothing opened on the erased seal", got)
	}
	if runs, _ := store.SimulationRuns(gocontext.Background(), first.Id); len(runs) != 1 {
		t.Errorf("the experiment holds %d runs, want 1", len(runs))
	}
}
