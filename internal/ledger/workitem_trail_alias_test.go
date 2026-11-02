package ledger_test

import (
	"context"
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
A card's trail joins its pull requests (#204, slice 2b).

Resolution is one level deep: rootOf(ref) is the alias's root, or the ref
itself. Asked by the issue, the trail holds the runs that claimed it and the
runs that claimed any pull request linked to it; asked by a pull request, it
is the root's trail, and says what it resolved from. Each run says how it is
there — its claim, or an alias — and which reference it claimed.

Aliases are read under the caller's own scopes, like runs: a link made by a
run the caller cannot read neither joins a trail nor resolves one, so the
trail reveals nothing about work in a scope the caller cannot see.
*/

// linkIn has a run that already holds root open alias and link it, in scope.
func linkIn(t *testing.T, s Store, id domain.RunID, agent domain.AgentID, scope domain.Scope, alias, root domain.WorkItemRef) {
	t.Helper()
	stamped := func(st domain.Step) domain.Step {
		st.AgentID, st.Scope = agent, scope
		return st
	}
	pr := domain.ToolReturnedPayload{Tool: "github-developer.create_pull_request", PullRequest: hosted(alias)}
	mustAppend(t, s, stamped(withPayload(t, step(id, domain.StepToolCalled),
		domain.ToolCalledPayload{Tool: pr.Tool, Effect: domain.EffectWrite})))
	proof := mustAppendSeq(t, s, stamped(withPayload(t, step(id, domain.StepToolReturned), pr)))
	mustAppend(t, s, stamped(linkStepOf(t, id, agent, linked(alias, root, proof))))
}

func trailFor(t *testing.T, s Store, filter domain.RunFilter, ref domain.WorkItemRef) domain.WorkItemTrail {
	t.Helper()
	got, err := s.WorkItemTrail(context.Background(), filter, &domain.RunFilter{}, ref, domain.WorkItemPage{})
	if err != nil {
		t.Fatalf("WorkItemTrail(%s): %v", ref, err)
	}
	return got
}

// The factory's shape: the Developer claims the issue and links its pull
// request; the Reviewer and QA claim the pull request.
func seedFactoryCard(t *testing.T, s Store) {
	t.Helper()
	trailRunIn(t, s, "run-dev", "developer", areaFactory, 0, card)
	linkIn(t, s, "run-dev", "developer", areaFactory, linkAlias, card)
	costed(t, s, "run-dev", "developer", areaFactory, 100)
	trailRunIn(t, s, "run-review", "reviewer", areaFactory, 10, linkAlias)
	approvalsOn(t, s, "run-review", "reviewer", areaFactory, true)
	costed(t, s, "run-review", "reviewer", areaFactory, 20)
	trailRunIn(t, s, "run-qa", "qa", areaFactory, 20, linkAlias)
	costed(t, s, "run-qa", "qa", areaFactory, 3)
	trailRunIn(t, s, "run-unrelated", "reviewer", areaFactory, 30, otherAlias)
}

type runLink struct {
	id   domain.RunID
	link domain.WorkItemLink
	ref  domain.WorkItemRef
}

func runLinks(runs []domain.WorkItemRun) []runLink {
	out := make([]runLink, len(runs))
	for i, r := range runs {
		out[i] = runLink{r.RunID, r.Link, r.Ref}
	}
	return out
}

func TestWorkItemTrail_byTheIssue_includesThePullRequestsReviewerAndQARuns(t *testing.T) {
	run(t, "the issue's trail joins its pull request", func(t *testing.T, s Store) {
		seedFactoryCard(t, s)

		got := trailFor(t, s, domain.RunFilter{}, card)

		want := []runLink{
			{"run-dev", domain.WorkItemLinkClaim, card},
			{"run-review", domain.WorkItemLinkAlias, linkAlias},
			{"run-qa", domain.WorkItemLinkAlias, linkAlias},
		}
		if !slices.Equal(runLinks(got.Runs), want) {
			t.Fatalf("runs = %+v, want %+v", runLinks(got.Runs), want)
		}
		if got.Ref != card || got.ResolvedFrom != "" {
			t.Fatalf("trail ref %q resolved from %q; want the issue itself, unresolved", got.Ref, got.ResolvedFrom)
		}
		reviewer := roleOf(t, got.Roles, "reviewer")
		if len(got.Roles) != 3 || reviewer.Runs != 1 || reviewer.Approvals.Approved != 1 ||
			reviewer.CostMicros == nil || *reviewer.CostMicros != 20 {
			t.Fatalf("roles = %+v; want three, the reviewer's run with its approval and cost", got.Roles)
		}
	})
}

func TestWorkItemTrail_byThePullRequest_resolvesToTheRootsTrail(t *testing.T) {
	run(t, "the pull request's trail is the issue's", func(t *testing.T, s Store) {
		seedFactoryCard(t, s)

		byPR := trailFor(t, s, domain.RunFilter{}, linkAlias)
		byIssue := trailFor(t, s, domain.RunFilter{}, card)

		if byPR.Ref != card || byPR.ResolvedFrom != linkAlias {
			t.Fatalf("trail by the pull request = ref %q from %q; want %q resolved from %q",
				byPR.Ref, byPR.ResolvedFrom, card, linkAlias)
		}
		if !slices.Equal(runLinks(byPR.Runs), runLinks(byIssue.Runs)) || len(byPR.Roles) != len(byIssue.Roles) {
			t.Fatalf("by PR %+v, by issue %+v; want the same trail", runLinks(byPR.Runs), runLinks(byIssue.Runs))
		}
	})
}

// A pull request nobody linked is its own reference with its own trail.
func TestWorkItemTrail_anUnlinkedPullRequest_isItsOwnTrail(t *testing.T) {
	run(t, "an unlinked pull request", func(t *testing.T, s Store) {
		seedFactoryCard(t, s)

		got := trailFor(t, s, domain.RunFilter{}, otherAlias)
		want := []runLink{{"run-unrelated", domain.WorkItemLinkClaim, otherAlias}}
		if got.Ref != otherAlias || got.ResolvedFrom != "" || !slices.Equal(runLinks(got.Runs), want) {
			t.Fatalf("trail = %q from %q, runs %+v; want its own runs, unresolved", got.Ref, got.ResolvedFrom, runLinks(got.Runs))
		}
	})
}

// The link is the Developer's, in factory; a caller who reads only cx —
// where a reviewer claimed the pull request — learns nothing of it: the
// pull request does not resolve, and its run does not join the issue.
func TestWorkItemTrail_aLinkTheCallerCannotRead_neitherJoinsNorResolves(t *testing.T) {
	run(t, "an unreadable link is no link", func(t *testing.T, s Store) {
		trailRunIn(t, s, "run-dev", "developer", areaFactory, 0, card)
		linkIn(t, s, "run-dev", "developer", areaFactory, linkAlias, card)
		trailRunIn(t, s, "run-review", "reviewer", areaCX, 10, linkAlias)
		cxOnly := domain.RunFilter{Scopes: []domain.Scope{areaCX}}

		byPR := trailFor(t, s, cxOnly, linkAlias)
		if byPR.Ref != linkAlias || byPR.ResolvedFrom != "" {
			t.Fatalf("cx caller's trail by the pull request = %q from %q; want it unresolved", byPR.Ref, byPR.ResolvedFrom)
		}
		if want := []runLink{{"run-review", domain.WorkItemLinkClaim, linkAlias}}; !slices.Equal(runLinks(byPR.Runs), want) {
			t.Fatalf("cx caller's runs = %+v, want %+v", runLinks(byPR.Runs), want)
		}
		if byIssue := trailFor(t, s, cxOnly, card); len(byIssue.Runs) != 0 || len(byIssue.Roles) != 0 {
			t.Fatalf("cx caller's trail by the issue = %+v; the unreadable link joined it", byIssue)
		}

		both := domain.RunFilter{Scopes: []domain.Scope{areaCX, areaFactory}}
		if got := trailFor(t, s, both, card); len(got.Runs) != 2 {
			t.Fatalf("a caller reading both areas sees %+v; want the developer and the reviewer", runLinks(got.Runs))
		}
	})
}

// Keyed per company: another company's link of the same pull request joins
// none of this company's trail.
func TestWorkItemTrail_anotherCompanysLink_joinsNothingHere(t *testing.T) {
	run(t, "links stay in their company", func(t *testing.T, s Store) {
		globex := domain.Scope{Company: "globex", Area: "factory"}
		trailRunIn(t, s, "run-g-dev", "developer", globex, 0, otherCard)
		linkIn(t, s, "run-g-dev", "developer", globex, linkAlias, otherCard)
		trailRunIn(t, s, "run-review", "reviewer", areaFactory, 10, linkAlias)

		if got := trailFor(t, s, domain.RunFilter{}, otherCard); !slices.Equal(trailIDs(got.Runs), []domain.RunID{"run-g-dev"}) {
			t.Fatalf("globex's card trail = %v; acme's reviewer joined it", trailIDs(got.Runs))
		}
		acme := domain.RunFilter{Scopes: []domain.Scope{{Company: "acme"}}}
		if got := trailFor(t, s, acme, linkAlias); got.ResolvedFrom != "" || got.Ref != linkAlias {
			t.Fatalf("acme's trail by the pull request = %q from %q; globex's link resolved it", got.Ref, got.ResolvedFrom)
		}
	})
}

// A simulated run's link projects nothing, so it joins nothing.
func TestWorkItemTrail_aSimulatedLink_joinsNothing(t *testing.T) {
	run(t, "a simulated link is no link", func(t *testing.T, s Store) {
		openSimulatedRun(t, s, "run-sim", "developer")
		if err := claim(t, s, "run-sim", "developer", card); err != nil {
			t.Fatalf("simulated claim: %v", err)
		}
		proof := returnPullRequest(t, s, "run-sim", "developer", linkAlias)
		if err := linkWith(t, s, "run-sim", "developer", linked(linkAlias, card, proof)); err != nil {
			t.Fatalf("simulated link: %v", err)
		}
		trailRunIn(t, s, "run-review", "reviewer", areaCX, 10, linkAlias)

		if got := trailFor(t, s, domain.RunFilter{}, card); len(got.Runs) != 0 {
			t.Fatalf("the issue's trail = %+v; a simulated link joined it", runLinks(got.Runs))
		}
		if got := trailFor(t, s, domain.RunFilter{}, linkAlias); got.ResolvedFrom != "" {
			t.Fatalf("a simulated link resolved the pull request to %q", got.Ref)
		}
	})
}
