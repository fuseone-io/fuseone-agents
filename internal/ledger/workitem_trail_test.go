package ledger_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
A card's trail, against both stores (#204, slice 1).

The runs that claimed the card, oldest claim first, and per role what they
asked people and cost. Narrowed by the caller's own scopes on every page —
a run the caller cannot read is absent, and so are its approvals and its
cost — and never a simulation.
*/

var (
	areaCX      = domain.Scope{Company: "acme", Area: "cx"}
	areaFactory = domain.Scope{Company: "acme", Area: "factory"}
	trailStart  = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
)

// trailRunIn opens a run of agent in scope at a minute past trailStart and
// claims ref at the same moment.
func trailRunIn(t *testing.T, s Store, id domain.RunID, agent domain.AgentID, scope domain.Scope, minute int, ref domain.WorkItemRef) {
	t.Helper()
	at := trailStart.Add(time.Duration(minute) * time.Minute)
	started := step(id, domain.StepRunStarted)
	started.AgentID, started.Scope, started.At = agent, scope, at
	mustAppend(t, s, started)
	claimed := claimStep(t, id, agent, ref)
	claimed.Scope, claimed.At = scope, at
	mustAppend(t, s, claimed)
}

// endRun finishes a run, which releases its card to the agent's next run.
func endRun(t *testing.T, s Store, id domain.RunID, agent domain.AgentID, scope domain.Scope) {
	t.Helper()
	finished := step(id, domain.StepRunFinished)
	finished.AgentID, finished.Scope = agent, scope
	mustAppend(t, s, finished)
}

// costed seals a step that spent micros, in the run's own scope and agent.
func costed(t *testing.T, s Store, id domain.RunID, agent domain.AgentID, scope domain.Scope, micros int64) {
	t.Helper()
	planned := step(id, domain.StepPlanned)
	planned.AgentID, planned.Scope = agent, scope
	planned.Cost = domain.Cost{Micros: micros}
	mustAppend(t, s, planned)
}

// approvalsOn seals approval steps on a run: requested, then decided each way.
func approvalsOn(t *testing.T, s Store, id domain.RunID, agent domain.AgentID, scope domain.Scope, decisions ...bool) {
	t.Helper()
	for _, approved := range decisions {
		req := step(id, domain.StepApprovalRequested)
		req.AgentID, req.Scope = agent, scope
		req.Payload = mustJSON(t, domain.ApprovalRequestedPayload{Tool: "github.issue_write", ArgsRef: "content:args-secret"})
		sealed, err := s.Append(context.Background(), req)
		if err != nil {
			t.Fatalf("request approval: %v", err)
		}
		dec := step(id, domain.StepApprovalDecided)
		dec.AgentID, dec.Scope = agent, scope
		dec.Payload = mustJSON(t, domain.ApprovalDecidedPayload{Approved: approved, By: "ana", AtSeq: sealed.Seq})
		mustAppend(t, s, dec)
	}
}

func trailOf(t *testing.T, s Store, filter domain.RunFilter, cost *domain.RunFilter, page domain.WorkItemPage) domain.WorkItemTrail {
	t.Helper()
	got, err := s.WorkItemTrail(context.Background(), filter, cost, card, page)
	if err != nil {
		t.Fatalf("WorkItemTrail: %v", err)
	}
	return got
}

func trailIDs(runs []domain.WorkItemRun) []domain.RunID {
	out := make([]domain.RunID, len(runs))
	for i, r := range runs {
		out[i] = r.RunID
	}
	return out
}

func roleOf(t *testing.T, roles []domain.WorkItemRole, agent domain.AgentID) domain.WorkItemRole {
	t.Helper()
	for _, r := range roles {
		if r.AgentID == agent {
			return r
		}
	}
	t.Fatalf("no role %s in %+v", agent, roles)
	return domain.WorkItemRole{}
}

func TestWorkItemTrail_listsTheCardsRunsOldestClaimFirstWithTheirRoles(t *testing.T) {
	run(t, "the card's runs and roles", func(t *testing.T, s Store) {
		trailRunIn(t, s, "run-dev", "developer", areaFactory, 20, card)
		trailRunIn(t, s, "run-triage", "triage", areaFactory, 0, card)
		trailRunIn(t, s, "run-other-card", "triage", areaFactory, 5, otherCard)
		approvalsOn(t, s, "run-dev", "developer", areaFactory, true, false, true)
		endRun(t, s, "run-triage", "triage", areaFactory)
		simulated := domain.RunID("run-sim")
		openSimulatedRun(t, s, simulated, "developer")
		if err := claim(t, s, simulated, "developer", card); err != nil {
			t.Fatalf("simulated claim: %v", err)
		}

		got := trailOf(t, s, domain.RunFilter{}, nil, domain.WorkItemPage{})

		if want := []domain.RunID{"run-triage", "run-dev"}; !slices.Equal(trailIDs(got.Runs), want) {
			t.Fatalf("runs = %v, want %v: the card's real runs, oldest claim first", trailIDs(got.Runs), want)
		}
		first := got.Runs[0]
		if first.AgentID != "triage" || first.Scope != areaFactory || first.Phase != "finished" ||
			first.Link != domain.WorkItemLinkClaim || first.EndedAt.IsZero() || !first.ClaimedAt.Equal(trailStart) {
			t.Errorf("first run = %+v", first)
		}
		dev := roleOf(t, got.Roles, "developer")
		want := domain.WorkItemApprovals{Requested: 3, Approved: 2, Refused: 1}
		if dev.Runs != 1 || dev.Approvals != want {
			t.Errorf("developer = %d runs, %+v; want 1 run, %+v", dev.Runs, dev.Approvals, want)
		}
		if got.Ref != card || got.Next != nil || len(got.Roles) != 2 {
			t.Errorf("trail = ref %q, next %+v, %d roles", got.Ref, got.Next, len(got.Roles))
		}
	})
}

// Contract 1: a run outside the caller's scopes is absent — not anonymised —
// and nothing of it counts: not its approvals, not its cost, not its run.
func TestWorkItemTrail_aRunOutsideTheCallersScopes_isAbsentFromRunsAndEveryAggregate(t *testing.T) {
	run(t, "unreadable runs count nowhere", func(t *testing.T, s Store) {
		trailRunIn(t, s, "run-visible", "developer", areaFactory, 0, card)
		endRun(t, s, "run-visible", "developer", areaFactory)
		trailRunIn(t, s, "run-hidden", "developer", areaCX, 1, card)
		approvalsOn(t, s, "run-hidden", "developer", areaCX, true, false)
		costed(t, s, "run-hidden", "developer", areaCX, 9_000)
		costed(t, s, "run-visible", "developer", areaFactory, 100)
		factory := domain.RunFilter{Scopes: []domain.Scope{areaFactory}}

		got := trailOf(t, s, factory, &factory, domain.WorkItemPage{})

		if ids := trailIDs(got.Runs); !slices.Equal(ids, []domain.RunID{"run-visible"}) {
			t.Fatalf("runs = %v, want only the readable one", ids)
		}
		dev := roleOf(t, got.Roles, "developer")
		if dev.Runs != 1 || dev.Approvals != (domain.WorkItemApprovals{}) {
			t.Errorf("developer = %d runs, %+v; the hidden run's approvals counted", dev.Runs, dev.Approvals)
		}
		if dev.CostMicros == nil || *dev.CostMicros != 100 {
			t.Errorf("developer cost = %v, want 100: the hidden run's cost counted", dev.CostMicros)
		}
	})
}

// Contract 2: without cost:read the cost is absent, never zero — and present
// when the caller may read it.
func TestWorkItemTrail_cost_presentOnlyWhereTheCallerMayReadIt(t *testing.T) {
	run(t, "cost only when allowed", func(t *testing.T, s Store) {
		trailRunIn(t, s, "run-a", "developer", areaFactory, 0, card)
		costed(t, s, "run-a", "developer", areaFactory, 250)
		factory := domain.RunFilter{Scopes: []domain.Scope{areaFactory}}
		elsewhere := domain.RunFilter{Scopes: []domain.Scope{areaCX}}

		for name, c := range map[string]struct {
			cost *domain.RunFilter
			want *int64
		}{
			"no cost:read":            {nil, nil},
			"cost:read elsewhere":     {&elsewhere, nil},
			"cost:read in its scopes": {&factory, ptrTo(int64(250))},
		} {
			role := roleOf(t, trailOf(t, s, factory, c.cost, domain.WorkItemPage{}).Roles, "developer")
			if (role.CostMicros == nil) != (c.want == nil) || (c.want != nil && *role.CostMicros != *c.want) {
				t.Errorf("%s: cost = %v, want %v", name, role.CostMicros, c.want)
			}
		}
	})
}

// Contract 5: walking every page yields every run once, in order, with no
// gap — including runs that share a claim instant across a page boundary.
func TestWorkItemTrail_walkingEveryPage_hasNoDuplicatesAndNoGaps(t *testing.T) {
	run(t, "keyset pages", func(t *testing.T, s Store) {
		var want []domain.RunID
		for i, id := range []domain.RunID{"run-1", "run-2", "run-3", "run-4", "run-5", "run-6", "run-7"} {
			trailRunIn(t, s, id, domain.AgentID("role-"+string(id)), areaFactory, i/3, card)
			want = append(want, id)
		}

		var seen []domain.RunID
		page := domain.WorkItemPage{Limit: 2}
		for pages := 0; ; pages++ {
			if pages > 10 {
				t.Fatalf("no end after %d pages: %v", pages, seen)
			}
			got := trailOf(t, s, domain.RunFilter{}, nil, page)
			if (page.After == nil) != (got.Roles != nil) {
				t.Fatalf("page %d: roles %v; want them on the first page only", pages, got.Roles)
			}
			seen = append(seen, trailIDs(got.Runs)...)
			if got.Next == nil {
				break
			}
			page.After = got.Next
		}
		if !slices.Equal(seen, want) {
			t.Fatalf("walked %v, want %v", seen, want)
		}
	})
}

// A full last page offers no next page: the store reads one run past it to
// know, rather than sending the reader to an empty page.
func TestWorkItemTrail_aPageThatEndsTheTrailExactly_offersNoNext(t *testing.T) {
	run(t, "exact last page", func(t *testing.T, s Store) {
		trailRunIn(t, s, "run-1", "triage", areaFactory, 0, card)
		trailRunIn(t, s, "run-2", "developer", areaFactory, 1, card)

		got := trailOf(t, s, domain.RunFilter{}, nil, domain.WorkItemPage{Limit: 2})
		if len(got.Runs) != 2 || got.Next != nil {
			t.Fatalf("runs %v next %+v; want both and no next", trailIDs(got.Runs), got.Next)
		}
	})
}

// The cursor carries no authority: a position taken under a wide filter,
// resumed under a narrow one, yields only what the narrow one reads.
func TestWorkItemTrail_aCursorFromAWideCaller_resumedNarrow_readsOnlyTheNarrowScope(t *testing.T) {
	run(t, "cursor carries no filter", func(t *testing.T, s Store) {
		trailRunIn(t, s, "run-1", "triage", areaFactory, 0, card)
		endRun(t, s, "run-1", "triage", areaFactory)
		trailRunIn(t, s, "run-2", "triage", areaCX, 1, card)
		trailRunIn(t, s, "run-3", "developer", areaCX, 2, card)
		endRun(t, s, "run-3", "developer", areaCX)
		trailRunIn(t, s, "run-4", "developer", areaFactory, 3, card)
		wide := trailOf(t, s, domain.RunFilter{}, nil, domain.WorkItemPage{Limit: 1})

		narrow := domain.RunFilter{Scopes: []domain.Scope{areaFactory}}
		got := trailOf(t, s, narrow, nil, domain.WorkItemPage{After: wide.Next, Limit: 10})

		if ids := trailIDs(got.Runs); !slices.Equal(ids, []domain.RunID{"run-4"}) {
			t.Fatalf("resumed narrow = %v, want only run-4", ids)
		}
	})
}

func TestWorkItemTrail_anUnclaimedCard_isAnEmptyTrailNotAnError(t *testing.T) {
	run(t, "empty trail", func(t *testing.T, s Store) {
		trailRunIn(t, s, "run-1", "triage", areaFactory, 0, otherCard)

		got := trailOf(t, s, domain.RunFilter{}, nil, domain.WorkItemPage{})
		if len(got.Runs) != 0 || len(got.Roles) != 0 || got.Roles == nil || got.Next != nil {
			t.Fatalf("trail = %+v; want no runs, an empty role list and no next", got)
		}
	})
}

// planter is a store a test can plant a divergent claim into.
type planter interface {
	PlantWorkItemClaim(ctx context.Context, runID domain.RunID, agent domain.AgentID,
		scope domain.Scope, ref domain.WorkItemRef) error
}

/*
Identity and cost come from the run, never from its claim (review round 1).

A claim recorded under another agent and scope than its run's — planted,
since the append refuses one — must not move the run on the trail: it is the
run's agent, the run's scope, and the run's scope that cost:read is judged
against. Read from the claim, a run in acme/cx by real-agent would show as
spoofed-agent in acme/factory, and its cost would be revealed to a caller
with cost:read on factory alone.
*/
func TestWorkItemTrail_aClaimRecordedUnderAnotherIdentity_readsAsTheRunsOwn(t *testing.T) {
	run(t, "identity from the run", func(t *testing.T, s Store) {
		started := step("run-real", domain.StepRunStarted)
		started.AgentID, started.Scope = "real-agent", areaCX
		mustAppend(t, s, started)
		costed(t, s, "run-real", "real-agent", areaCX, 987)
		if err := s.(planter).PlantWorkItemClaim(context.Background(), "run-real", "spoofed-agent", areaFactory, card); err != nil {
			t.Fatalf("plant: %v", err)
		}
		factoryCost := domain.RunFilter{Scopes: []domain.Scope{areaFactory}}

		got := trailOf(t, s, domain.RunFilter{}, &factoryCost, domain.WorkItemPage{})

		if len(got.Runs) != 1 || got.Runs[0].AgentID != "real-agent" || got.Runs[0].Scope != areaCX {
			t.Fatalf("runs = %+v; want the run's own agent and scope", got.Runs)
		}
		if len(got.Roles) != 1 || got.Roles[0].AgentID != "real-agent" || got.Roles[0].CostMicros != nil {
			t.Fatalf("roles = %+v; want real-agent with no cost (cost:read is on factory only)", got.Roles)
		}
		cx := trailOf(t, s, domain.RunFilter{Scopes: []domain.Scope{areaFactory}}, &factoryCost, domain.WorkItemPage{})
		if len(cx.Runs) != 0 {
			t.Fatalf("a factory reader sees %+v; the run is cx's", cx.Runs)
		}
	})
}

func ptrTo[T any](v T) *T { return &v }
