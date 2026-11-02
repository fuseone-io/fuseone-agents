package ledger_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
A run's inherited work item, against both stores (#204, slice 3).

A run an event or a signed GitHub delivery opened carries the card it is
attributed to on its run_started. The ledger validates it there, and the
card's trail then shows the run by how it was opened. Attribution only: it
never holds the card, never reaches run_work_items, and never blocks a claim.
*/

func delivered(ref domain.WorkItemRef) *domain.RunWorkItem {
	return &domain.RunWorkItem{Ref: ref, Source: domain.WorkItemFromGitHubDelivery}
}

func fromEvent(ref domain.WorkItemRef, source domain.RunID) *domain.RunWorkItem {
	return &domain.RunWorkItem{Ref: ref, Source: domain.WorkItemFromEvent, FromRun: source}
}

// inheritedStart is a run's opening step carrying item.
func inheritedStart(
	t *testing.T, id domain.RunID, agent domain.AgentID, scope domain.Scope, minute int, item *domain.RunWorkItem,
) domain.Step {
	t.Helper()
	started := step(id, domain.StepRunStarted)
	started.AgentID, started.Scope = agent, scope
	started.At = trailStart.Add(time.Duration(minute) * time.Minute)
	started.Payload = mustJSON(t, domain.RunStartedPayload{Trigger: "event", WorkItem: item})
	return started
}

// inheritIn opens a run of agent in scope, at a minute past trailStart,
// carrying item.
func inheritIn(
	t *testing.T, s Store, id domain.RunID, agent domain.AgentID, scope domain.Scope, minute int, item *domain.RunWorkItem,
) {
	t.Helper()
	mustAppend(t, s, inheritedStart(t, id, agent, scope, minute, item))
}

func TestWorkItemOrigin_aRunOpenedWithAWorkItem_isOnTheCardsTrailAndCounted(t *testing.T) {
	run(t, "an inherited run is on the trail", func(t *testing.T, s Store) {
		trailRunIn(t, s, "run-dev", "developer", areaFactory, 0, card)
		inheritIn(t, s, "run-review", "reviewer", areaFactory, 5, fromEvent(card, "run-dev"))
		costed(t, s, "run-review", "reviewer", areaFactory, 40)
		approvalsOn(t, s, "run-review", "reviewer", areaFactory, true)
		inheritIn(t, s, "run-hook", "triage", areaFactory, 9, delivered(card))
		inheritIn(t, s, "run-elsewhere", "triage", areaFactory, 12, delivered(otherCard))

		got := trailOf(t, s, domain.RunFilter{}, &domain.RunFilter{}, domain.WorkItemPage{})

		want := []runLink{
			{"run-dev", domain.WorkItemLinkClaim, card},
			{"run-review", domain.WorkItemLinkEvent, card},
			{"run-hook", domain.WorkItemLinkGitHubDelivery, card},
		}
		if !slices.Equal(runLinks(got.Runs), want) {
			t.Fatalf("runs = %+v, want %+v", runLinks(got.Runs), want)
		}
		if at := got.Runs[1].ClaimedAt; !at.Equal(trailStart.Add(5 * time.Minute)) {
			t.Errorf("the inherited run joined at %v, want the moment it opened", at)
		}
		// Counted as a run of the role, with what it asked people — but its
		// spend is the card it claimed, not this one: cost is additive.
		reviewer := roleOf(t, got.Roles, "reviewer")
		if reviewer.Runs != 1 || reviewer.Approvals.Approved != 1 || reviewer.CostMicros == nil || *reviewer.CostMicros != 0 {
			t.Fatalf("reviewer role = %+v; want the run and its approval counted, its cost not", reviewer)
		}
	})
}

/*
A card's cost is what the runs attributed to it by a claim spent, so card
totals add up: a run that inherits one card, claims another and spends 100
is 100 on the card it claimed and 0 on the one it inherited — summed over
both trails, its cost once.
*/
func TestWorkItemOrigin_costOverBothTrails_isTheRunsCostOnce(t *testing.T) {
	run(t, "cost is additive across trails", func(t *testing.T, s Store) {
		inheritIn(t, s, "run-x", "developer", areaFactory, 0, delivered(card))
		other := claimStep(t, "run-x", "developer", otherCard)
		other.Scope = areaFactory
		mustAppend(t, s, other)
		costed(t, s, "run-x", "developer", areaFactory, 100)

		var total int64
		for _, ref := range []domain.WorkItemRef{card, otherCard} {
			role := roleOf(t, trailFor(t, s, domain.RunFilter{}, ref).Roles, "developer")
			if role.CostMicros == nil {
				t.Fatalf("%s: cost absent for a caller with cost:read everywhere", ref)
			}
			total += *role.CostMicros
		}
		if total != 100 {
			t.Fatalf("cost summed over both trails = %d, want the run's 100 once", total)
		}
	})
}

// The fence Kleber set for this slice: inheritance is attribution, never a
// hold. A run of the developer that only inherited the card stays open while
// another run of the same developer claims it.
func TestWorkItemOrigin_neverBlocksAClaim(t *testing.T) {
	run(t, "inheritance never holds the card", func(t *testing.T, s Store) {
		inheritIn(t, s, "run-inherits", "developer", areaFactory, 0, fromEvent(card, "run-triage"))
		openRunOf(t, s, "run-claims", "developer")
		if err := claim(t, s, "run-claims", "developer", card); err != nil {
			t.Fatalf("a claim beside an open run that inherited the card = %v, want success", err)
		}
		// The inheriting run itself is free to claim whatever it likes.
		own := claimStep(t, "run-inherits", "developer", otherCard)
		own.Scope = areaFactory
		if _, err := s.Append(context.Background(), own); err != nil {
			t.Fatalf("the inheriting run's own claim = %v, want success", err)
		}
		got, err := s.WorkItemCost(context.Background(), domain.RunFilter{}, card)
		if err != nil || got.Runs != 1 {
			t.Fatalf("WorkItemCost = %+v, %v; FinOps counts claims only", got, err)
		}
	})
}

/*
A run that inherits one card and claims another is on both trails, once each,
and each row says where else the run is: on the card it claimed, the
inheritance; on the card it inherited, the claim.
*/
func TestWorkItemOrigin_inheritsOneCardClaimsAnother_isOnBothFlagged(t *testing.T) {
	run(t, "dual attribution", func(t *testing.T, s Store) {
		inheritIn(t, s, "run-x", "developer", areaFactory, 0, delivered(card))
		other := claimStep(t, "run-x", "developer", otherCard)
		other.Scope = areaFactory
		mustAppend(t, s, other)

		inherited := trailFor(t, s, domain.RunFilter{}, card)
		claimed := trailFor(t, s, domain.RunFilter{}, otherCard)

		if len(inherited.Runs) != 1 || inherited.Runs[0].Link != domain.WorkItemLinkGitHubDelivery ||
			inherited.Runs[0].ClaimedRef != otherCard || inherited.Runs[0].InheritedRef != "" {
			t.Fatalf("the inherited card's trail = %+v", inherited.Runs)
		}
		if len(claimed.Runs) != 1 || claimed.Runs[0].Link != domain.WorkItemLinkClaim ||
			claimed.Runs[0].InheritedRef != card || claimed.Runs[0].ClaimedRef != "" {
			t.Fatalf("the claimed card's trail = %+v", claimed.Runs)
		}
	})
}

// Inherited and claimed on one card, a run is on its trail once, by its
// claim — the stronger of the two — and counted once.
func TestWorkItemOrigin_inheritsAndClaimsTheSameCard_isOnItsTrailOnceAsTheClaim(t *testing.T) {
	run(t, "one row per run", func(t *testing.T, s Store) {
		inheritIn(t, s, "run-x", "developer", areaFactory, 0, fromEvent(card, "run-triage"))
		claimed := claimStep(t, "run-x", "developer", card)
		claimed.Scope, claimed.At = areaFactory, trailStart.Add(3*time.Minute)
		mustAppend(t, s, claimed)

		got := trailFor(t, s, domain.RunFilter{}, card)

		if want := []runLink{{"run-x", domain.WorkItemLinkClaim, card}}; !slices.Equal(runLinks(got.Runs), want) {
			t.Fatalf("runs = %+v, want %+v", runLinks(got.Runs), want)
		}
		if got.Runs[0].InheritedRef != card {
			t.Errorf("inherited = %q, want the inheritance flagged on the claim", got.Runs[0].InheritedRef)
		}
		if role := roleOf(t, got.Roles, "developer"); role.Runs != 1 {
			t.Fatalf("developer runs = %d, want the run counted once", role.Runs)
		}
	})
}

// Inheriting a pull request linked to the card puts the run on the card's
// trail, resolved by the link like any claim of it: nothing is resolved when
// the inheritance is written.
func TestWorkItemOrigin_aLinkedPullRequest_resolvesToTheCard(t *testing.T) {
	run(t, "inheritance through an alias", func(t *testing.T, s Store) {
		trailRunIn(t, s, "run-dev", "developer", areaFactory, 0, card)
		linkIn(t, s, "run-dev", "developer", areaFactory, linkAlias, card)
		inheritIn(t, s, "run-hook", "reviewer", areaFactory, 10, delivered(linkAlias))

		got := trailFor(t, s, domain.RunFilter{}, linkAlias)

		want := []runLink{
			{"run-dev", domain.WorkItemLinkClaim, card},
			{"run-hook", domain.WorkItemLinkGitHubDelivery, linkAlias},
		}
		if got.Ref != card || !slices.Equal(runLinks(got.Runs), want) {
			t.Fatalf("trail of %s = %s %+v, want %s %+v", linkAlias, got.Ref, runLinks(got.Runs), card, want)
		}
	})
}

func TestWorkItemOrigin_aRunOutsideTheCallersScopes_isAbsent(t *testing.T) {
	run(t, "inherited runs are read under the caller's scopes", func(t *testing.T, s Store) {
		inheritIn(t, s, "run-cx", "triage", areaCX, 0, delivered(card))
		costed(t, s, "run-cx", "triage", areaCX, 70)

		got := trailOf(t, s, domain.RunFilter{Scopes: []domain.Scope{areaFactory}}, nil, domain.WorkItemPage{})

		if len(got.Runs) != 0 || len(got.Roles) != 0 {
			t.Fatalf("trail = %+v; a run the caller cannot read is on it", got)
		}
	})
}

func TestWorkItemOrigin_walkingMixedPages_hasNoDuplicatesAndNoGaps(t *testing.T) {
	run(t, "keyset over claims and inheritances", func(t *testing.T, s Store) {
		var want []domain.RunID
		for i, id := range []domain.RunID{"run-a", "run-b", "run-c", "run-d", "run-e"} {
			if i%2 == 0 {
				trailRunIn(t, s, id, domain.AgentID("role-"+id), areaFactory, 1, card)
			} else {
				inheritIn(t, s, id, domain.AgentID("role-"+id), areaFactory, 1, delivered(card))
			}
			want = append(want, id)
		}
		var seen []domain.RunID
		page := domain.WorkItemPage{Limit: 2}
		for range 5 {
			got := trailOf(t, s, domain.RunFilter{}, nil, page)
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

func TestWorkItemOrigin_invalid_refusedAndStoresNothing(t *testing.T) {
	cases := map[string]*domain.RunWorkItem{
		"non-canonical":     {Ref: "github:FuseOne/x#1", Source: domain.WorkItemFromGitHubDelivery},
		"unknown source":    {Ref: card, Source: "claim"},
		"event, no run":     {Ref: card, Source: domain.WorkItemFromEvent},
		"delivery, a run":   {Ref: card, Source: domain.WorkItemFromGitHubDelivery, FromRun: "run-0"},
		"delivery, a board": {Ref: "ghproject:PVTI_1", Source: domain.WorkItemFromGitHubDelivery},
	}
	run(t, "an invalid inheritance is refused", func(t *testing.T, s Store) {
		for name, item := range cases {
			id := domain.RunID("run-" + name)
			_, err := s.Append(context.Background(), inheritedStart(t, id, "triage", areaFactory, 0, item))
			if !errors.Is(err, domain.ErrRunWorkItemInvalid) {
				t.Errorf("%s: %v, want ErrRunWorkItemInvalid", name, err)
			}
			if _, err := s.Head(context.Background(), id); err == nil {
				t.Errorf("%s: the run exists", name)
			}
		}
	})
}

// An inheritance is what opened the run, so it is sealed on the opening step
// alone; a later run_started cannot attribute a run already going.
func TestWorkItemOrigin_onALaterStep_refused(t *testing.T) {
	run(t, "an inheritance after the start is refused", func(t *testing.T, s Store) {
		openRunOf(t, s, "run-x", "triage")
		later := inheritedStart(t, "run-x", "triage", areaCX, 0, delivered(card))
		if _, err := s.Append(context.Background(), later); !errors.Is(err, domain.ErrRunWorkItemInvalid) {
			t.Fatalf("a later inheritance = %v, want ErrRunWorkItemInvalid", err)
		}
		if got := trailFor(t, s, domain.RunFilter{}, card); len(got.Runs) != 0 {
			t.Fatalf("trail = %+v", got.Runs)
		}
	})
}

func TestWorkItemOrigin_simulatedRun_isNotOnTheTrail(t *testing.T) {
	run(t, "a simulation inherits nothing durable", func(t *testing.T, s Store) {
		started := inheritedStart(t, "run-sim", "triage", areaFactory, 0, delivered(card))
		started.Payload = mustJSON(t, domain.RunStartedPayload{
			Trigger: "simulation", Simulated: true, Simulation: "sim-1", WorkItem: delivered(card),
		})
		mustAppend(t, s, started)
		if got := trailFor(t, s, domain.RunFilter{}, card); len(got.Runs) != 0 || len(got.Roles) != 0 {
			t.Fatalf("trail = %+v; a simulation is on it", got)
		}
	})
}

// Rolling upgrade: a start sealed before this release carries no work item,
// and the run is on no card's trail by it.
func TestWorkItemOrigin_anOldShapeStart_inheritsNothing(t *testing.T) {
	run(t, "an old-shape start", func(t *testing.T, s Store) {
		started := step("run-old", domain.StepRunStarted)
		started.AgentID, started.Scope = "triage", areaFactory
		started.Payload = []byte(`{"trigger":"event","chain":{"root":"run-0","hop":1}}`)
		mustAppend(t, s, started)
		if got := trailFor(t, s, domain.RunFilter{}, card); len(got.Runs) != 0 {
			t.Fatalf("trail = %+v", got.Runs)
		}
	})
}
