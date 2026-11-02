package httpapi

import (
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
GET /work-items/trail joins a card's pull requests (#204, slice 2b): asked by
the pull request, it answers the card's trail and says what it resolved from;
each run says whether it is there by its claim or by an alias, and which
reference it claimed.
*/

const trailPR = "github:fuseone-io/fuseone-agents#240"

// linkPR has the run that holds trailCard open trailPR and link it.
func (s trailSeed) linkPR(id domain.RunID, agent domain.AgentID, scope domain.Scope) {
	s.t.Helper()
	tool := domain.ToolID("github-developer.create_pull_request")
	s.step(id, agent, scope, domain.StepToolCalled, domain.ToolCalledPayload{Tool: tool, Effect: domain.EffectWrite})
	proof := s.step(id, agent, scope, domain.StepToolReturned, domain.ToolReturnedPayload{Tool: tool,
		PullRequest: &domain.PullRequestStamp{Ref: trailPR, Via: domain.ProvenanceHostedGitHubApp}})
	s.step(id, agent, scope, domain.StepWorkItemLinked, domain.WorkItemLinkedPayload{
		Alias: trailPR, Root: trailCard,
		Proof: domain.WorkItemLinkProof{Kind: domain.StepToolReturned, Seq: proof.Seq},
	})
}

func factorySeed(t *testing.T) trailSeed {
	t.Helper()
	seed := newTrailSeed(t)
	seed.claim("run-dev", "developer", trailFactory, trailCard, 10)
	seed.linkPR("run-dev", "developer", trailFactory)
	seed.claim("run-review", "reviewer", trailFactory, trailPR, 5)
	return seed
}

type servedLink struct{ run, link, ref string }

func servedLinks(p openapi.WorkItemTrail) []servedLink {
	out := []servedLink{}
	for _, r := range p.Runs {
		out = append(out, servedLink{r.RunId, string(r.Link), r.Ref})
	}
	return out
}

func TestGetWorkItemTrail_byTheIssue_servesThePullRequestsRunsAsAliases(t *testing.T) {
	t.Parallel()
	page := trailGet(t, trailServer(factorySeed(t)), companyAuditor(), trailRequest(trailCard, nil, 0))

	want := []servedLink{{"run-dev", "claim", trailCard}, {"run-review", "alias", trailPR}}
	if !slices.Equal(servedLinks(page), want) || page.Ref != trailCard || page.ResolvedFrom != nil {
		t.Fatalf("trail = ref %q from %v, runs %+v; want %+v unresolved", page.Ref, page.ResolvedFrom, servedLinks(page), want)
	}
}

func TestGetWorkItemTrail_byThePullRequest_servesTheIssuesTrailAndSaysSo(t *testing.T) {
	t.Parallel()
	server := trailServer(factorySeed(t))

	byPR := trailGet(t, server, companyAuditor(), trailRequest("github:FuseOne-IO/fuseone-agents#240", nil, 0))
	byIssue := trailGet(t, server, companyAuditor(), trailRequest(trailCard, nil, 0))

	if byPR.Ref != trailCard || byPR.ResolvedFrom == nil || *byPR.ResolvedFrom != trailPR {
		t.Fatalf("trail by the pull request = ref %q from %v; want %q from %q", byPR.Ref, byPR.ResolvedFrom, trailCard, trailPR)
	}
	if !slices.Equal(servedLinks(byPR), servedLinks(byIssue)) {
		t.Fatalf("by PR %+v, by issue %+v; want one trail", servedLinks(byPR), servedLinks(byIssue))
	}
}

// The field is absent, not empty, when nothing resolved: the body of an
// unlinked reference is the body it always was.
func TestGetWorkItemTrail_unresolved_carriesNoResolvedFromField(t *testing.T) {
	t.Parallel()
	resp, err := trailServer(factorySeed(t)).GetWorkItemTrail(companyAuditor(), trailRequest(trailCard, nil, 0))
	if err != nil {
		t.Fatalf("GetWorkItemTrail: %v", err)
	}
	if body := trailJSON(t, resp); strings.Contains(string(body), "resolvedFrom") {
		t.Fatalf("body = %s; an unresolved trail names no resolvedFrom", body)
	}
}
