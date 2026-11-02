package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/tools"
	"github.com/fuseone/agents/internal/workitem"
)

/*
A card's trail joins its pull request, from the entries serve and worker use
(#204, slice 2b).

A Developer run claims an issue, opens a pull request with a create_pull_request
a fake GitHub server answers, and links it; a Reviewer run then claims the pull
request. Both runs are driven by the engine over the worker's own deps — the
Postgres ledger openStore returns, the Gate, the content store, and the
work-item layer stacked outermost — with the GitHub server below it replaced
by the fake. The trail is then read over HTTP through the router serve mounts.

Each half has its own failure that a unit test with a fake would miss: a
worker whose outermost layer forgot the link would hand it to the server (the
fake fails the test if asked); an engine that did not record what the
create_pull_request answered named would park the link for a person; a ledger
that did not project the alias, or a trail that did not join it, would show
the issue with its Developer alone.
*/
func TestWorkItemLinkJourney_developerLinksItsPullRequest_theIssuesTrailShowsTheReviewer(t *testing.T) {
	dsn := testdb.Required(t)
	ctx := context.Background()
	store, err := openStore(ctx, dsn)
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	nonce := time.Now().UnixNano()
	repo := fmt.Sprintf("fuseone-io/journey-%d", nonce)
	issue := domain.WorkItemRef("github:" + repo + "#204")
	pr := domain.WorkItemRef("github:" + repo + "#240")
	github := &fakeGitHub{t: t, answer: fmt.Sprintf(`{"id":"9","url":"https://github.com/%s/pull/240"}`, repo)}
	scope := domain.Scope{Company: "acme", Area: "factory"}

	developer := journeyRunner(t, store, github, scope,
		claimOf(issue),
		engine.Proposal{Tool: createPullRequest, Args: []byte(`{"title":"fix","head":"factory/204-fix","base":"main"}`)},
		engine.Proposal{Tool: domain.ToolWorkItemLink, Args: refArgs(pr)},
	)
	devRun := domain.RunID(fmt.Sprintf("run-%d-developer", nonce))
	devStart := developer.open(t, devRun, "factory-developer")
	developer.advance(t, devStart, 2) // the claim; create_pull_request parks
	developer.approveLatest(t, devRun)
	developer.advance(t, devStart, 2) // the approved create_pull_request; the link
	if len(github.asked) != 1 || github.asked[0] != createPullRequest {
		t.Fatalf("the GitHub server was asked %v; only create_pull_request may reach it", github.asked)
	}
	requireLinked(t, store, devRun, pr, issue)

	reviewer := journeyRunner(t, store, github, scope, claimOf(pr))
	reviewRun := domain.RunID(fmt.Sprintf("run-%d-reviewer", nonce))
	reviewer.advance(t, reviewer.open(t, reviewRun, "factory-reviewer"), 1)

	route := apiHandlerFor(ledgerAPI(store, "test"))
	for _, asked := range []domain.WorkItemRef{issue, pr} {
		code, body := getTrail(t, route, factoryApprover(), url.Values{"ref": {string(asked)}})
		if code != http.StatusOK {
			t.Fatalf("GET trail of %s = %d: %s", asked, code, body)
		}
		var trail openapi.WorkItemTrail
		if err := json.Unmarshal(body, &trail); err != nil {
			t.Fatalf("decode: %v", err)
		}
		var got []string
		for _, r := range trail.Runs {
			got = append(got, r.RunId+" "+string(r.Link)+" "+r.Ref)
		}
		want := []string{string(devRun) + " claim " + string(issue), string(reviewRun) + " alias " + string(pr)}
		if trail.Ref != string(issue) || !slices.Equal(got, want) {
			t.Fatalf("trail asked by %s = ref %q, runs %v; want the issue's, with %v", asked, trail.Ref, got, want)
		}
	}
}

const createPullRequest = domain.ToolID("github-developer.create_pull_request")

func claimOf(ref domain.WorkItemRef) engine.Proposal {
	return engine.Proposal{Tool: domain.ToolWorkItemClaim, Args: refArgs(ref)}
}

func refArgs(ref domain.WorkItemRef) []byte {
	raw, _ := json.Marshal(map[string]string{"ref": string(ref)})
	return raw
}

// fakeGitHub is the GitHub server below the work-item layer: it opens a pull
// request, and it fails the test if it is asked anything about the platform's
// own tools.
type fakeGitHub struct {
	t       *testing.T
	answer  string
	content engine.ContentStore
	asked   []domain.ToolID
}

func (g *fakeGitHub) Effect(id domain.ToolID) (domain.Effect, bool) {
	if id.Coordination() {
		g.t.Errorf("the GitHub server was asked to classify %s", id)
	}
	return domain.EffectWrite, id == createPullRequest
}

func (g *fakeGitHub) Dedupe(domain.ToolID) (domain.ToolDedupe, bool) {
	return domain.ToolDedupe{}, false
}

func (g *fakeGitHub) Reserve(_ context.Context, call engine.Call) error {
	if call.Tool.Coordination() {
		g.t.Errorf("the GitHub server was asked to reserve %s", call.Tool)
	}
	return nil
}

func (g *fakeGitHub) Invoke(ctx context.Context, call engine.Call) (engine.ToolResult, error) {
	g.asked = append(g.asked, call.Tool)
	body := []byte(g.answer)
	ref, err := g.content.Put(ctx, call.RunID, call.Seq, body)
	if err != nil {
		return engine.ToolResult{}, err
	}
	return engine.ToolResult{ResultRef: ref, ResultDigest: engine.ResultDigest(body), ResultBytes: int64(len(body)),
		Provenance: domain.ProvenanceHostedGitHubApp}, nil
}

type journeyPlanner struct{ proposals []engine.Proposal }

func (p *journeyPlanner) Plan(context.Context, engine.PlanInput) (engine.Proposal, error) {
	if len(p.proposals) == 0 {
		return engine.Proposal{Done: true, Outcome: "done"}, nil
	}
	next := p.proposals[0]
	p.proposals = p.proposals[1:]
	return next, nil
}

type journey struct {
	runner  *engine.Runner
	store   Store
	content engine.ContentStore
	scope   domain.Scope
	// stage is copilot when unset; pack adds tools beside the work-item ones.
	stage domain.Stage
	pack  []domain.ToolID
}

// journeyRunner is the worker's own deps — the Gate, the store, the content
// store and the work-item layer outermost — over the fake GitHub server.
func journeyRunner(t *testing.T, store Store, github *fakeGitHub, scope domain.Scope, proposals ...engine.Proposal) journey {
	t.Helper()
	content := engine.NewMemoryContent()
	if github.content != nil {
		content = github.content.(*engine.MemoryContent)
	}
	github.content = content
	p := &workerParts{store: store, content: content, catalog: tools.NewCatalog(content)}
	deps := p.deps(gate.New(), nil)
	layer := workitem.NewLayer(github, github)
	deps.Tools, deps.Catalog = layer, layer
	deps.Schemas, deps.Connections = nil, nil
	deps.Planner = &journeyPlanner{proposals: proposals}
	return journey{runner: engine.NewRunner(deps), store: store, content: content, scope: scope}
}

// open starts a run with labels, or — given none — tainted by an untrusted
// read, the shape a factory run has.
func (j journey) open(t *testing.T, id domain.RunID, agent domain.AgentID, labels ...domain.Labels) engine.Start {
	t.Helper()
	stage := j.stage
	if stage == "" {
		stage = domain.StageCopilot
	}
	opened := domain.ScopeLabels(j.scope).Union(domain.NewLabels(domain.LabelUntrusted))
	if len(labels) > 0 {
		opened = labels[0]
	}
	start := engine.Start{
		RunID: id, Scope: j.scope, AgentID: agent, VersionID: "v1",
		Pack:   gate.NewPack(append([]domain.ToolID{domain.ToolWorkItemClaim, domain.ToolWorkItemLink, createPullRequest}, j.pack...)...),
		Budget: domain.Budget{Micros: 1_000_000, ToolCalls: 20, Steps: 50},
		Stage:  stage,
	}
	input, err := j.content.Put(context.Background(), id, domain.FirstSeq, []byte("pick a card"))
	if err != nil {
		t.Fatalf("store input: %v", err)
	}
	if _, err := j.store.Append(context.Background(), domain.Step{
		RunID: id, Kind: domain.StepRunStarted, Scope: j.scope, AgentID: agent, VersionID: "v1",
		Labels:  opened,
		Payload: mustJSONBytes(t, domain.RunStartedPayload{Trigger: "cron", InputRef: input}),
	}); err != nil {
		t.Fatalf("open %s: %v", id, err)
	}
	return start
}

func (j journey) advance(t *testing.T, start engine.Start, turns int) {
	t.Helper()
	for range turns {
		if _, err := j.runner.Advance(context.Background(), start); err != nil {
			t.Fatalf("Advance %s: %v", start.RunID, err)
		}
	}
}

// approveLatest answers the run's open approval request, as the console does.
func (j journey) approveLatest(t *testing.T, id domain.RunID) {
	t.Helper()
	head, err := j.store.Head(context.Background(), id)
	if err != nil || head.Kind != domain.StepApprovalRequested {
		t.Fatalf("head of %s = %+v, %v; want an open approval request", id, head, err)
	}
	if _, err := j.store.Append(context.Background(), domain.Step{
		RunID: id, Kind: domain.StepApprovalDecided, Scope: head.Scope, AgentID: head.AgentID, VersionID: "v1",
		Payload: mustJSONBytes(t, domain.ApprovalDecidedPayload{Approved: true, By: "usr_ana", AtSeq: head.Seq}),
	}); err != nil {
		t.Fatalf("approve: %v", err)
	}
}

func requireLinked(t *testing.T, store Store, id domain.RunID, alias, root domain.WorkItemRef) {
	t.Helper()
	steps, err := store.Read(context.Background(), id, domain.FirstSeq)
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	var kinds []domain.StepKind
	for _, st := range steps {
		kinds = append(kinds, st.Kind)
		// One request: create_pull_request's. The proven link asks nobody.
		if st.Kind == domain.StepApprovalRequested && slices.Contains(kinds[:len(kinds)-1], domain.StepApprovalRequested) {
			t.Fatalf("the proven link asked for approval: %v", kinds)
		}
		if st.Kind != domain.StepWorkItemLinked {
			continue
		}
		var p domain.WorkItemLinkedPayload
		if err := json.Unmarshal(st.Payload, &p); err != nil || p.Alias != alias || p.Root != root ||
			p.Proof.Kind != domain.StepToolReturned {
			t.Fatalf("link = %+v (%v); want %s to %s proved by the create_pull_request", p, err, alias, root)
		}
		return
	}
	t.Fatalf("%s sealed no link: %v", id, kinds)
}

func mustJSONBytes(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}
