package domain_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
The ledger's weekly measures (#201): the week is the ISO week in UTC, the
window is whole weeks ending with the current one, and a row's cost is all
of its scopes or none of them.
*/

var (
	saoPaulo = time.FixedZone("BRT", -3*60*60)
	// mondayNov2 is the Monday that starts the ISO week of 2026-11-03.
	mondayNov2 = time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
)

func TestWeekOf_isTheMondayAtMidnightUTCThatStartsTheISOWeek(t *testing.T) {
	t.Parallel()
	lastInstant := mondayNov2.Add(-time.Microsecond)
	cases := []struct {
		name string
		at   time.Time
		want time.Time
	}{
		{"monday midnight starts its own week", mondayNov2, mondayNov2},
		{"sunday's last microsecond belongs to the week before", lastInstant, mondayNov2.AddDate(0, 0, -7)},
		{"the middle of the week", time.Date(2026, 11, 5, 13, 30, 0, 0, time.UTC), mondayNov2},
		{"sunday afternoon", time.Date(2026, 11, 8, 15, 0, 0, 0, time.UTC), mondayNov2},
		// Sunday 22:00 in São Paulo is Monday 01:00 UTC: the next week, by UTC.
		{"a local sunday that is a UTC monday", time.Date(2026, 11, 1, 22, 0, 0, 0, saoPaulo), mondayNov2},
		// Monday 00:30 in São Paulo is still Monday 03:30 UTC: the same week.
		{"a local monday", time.Date(2026, 11, 2, 0, 30, 0, 0, saoPaulo), mondayNov2},
		// Sunday 20:00 in São Paulo is Sunday 23:00 UTC: the week before.
		{"a local sunday that is a UTC sunday", time.Date(2026, 11, 1, 20, 0, 0, 0, saoPaulo), mondayNov2.AddDate(0, 0, -7)},
		{"across a year", time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC), time.Date(2026, 12, 28, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		got := domain.WeekOf(c.at)
		if !got.Equal(c.want) || got.Location() != time.UTC {
			t.Errorf("%s: WeekOf(%s) = %s, want %s in UTC", c.name, c.at, got, c.want)
		}
	}
}

func TestMeasureWindow_isWholeWeeksEndingWithTheCurrentOne(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, saoPaulo)

	got, err := domain.MeasureWindow(now, 8)
	if err != nil {
		t.Fatalf("MeasureWindow: %v", err)
	}
	wantTo := mondayNov2.AddDate(0, 0, 7)
	if !got.To.Equal(wantTo) || !got.From.Equal(wantTo.AddDate(0, 0, -56)) {
		t.Fatalf("window = [%s, %s), want [%s, %s)", got.From, got.To, wantTo.AddDate(0, 0, -56), wantTo)
	}
	if got.From.Location() != time.UTC || got.To.Location() != time.UTC {
		t.Errorf("window is not in UTC: [%s, %s)", got.From, got.To)
	}
}

func TestMeasureWindow_weeksOutsideOneToTwentySix_refused(t *testing.T) {
	t.Parallel()
	for _, weeks := range []int{-1, 0, 27, 1000} {
		if _, err := domain.MeasureWindow(mondayNov2, weeks); !errors.Is(err, domain.ErrMeasureWeeks) {
			t.Errorf("weeks %d: err = %v, want ErrMeasureWeeks", weeks, err)
		}
	}
	for _, weeks := range []int{1, 26} {
		if _, err := domain.MeasureWindow(mondayNov2, weeks); err != nil {
			t.Errorf("weeks %d: %v", weeks, err)
		}
	}
}

func TestWeekComplete_falseForTheCurrentWeekTrueOnceItEnded(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	if domain.WeekComplete(mondayNov2, now) {
		t.Error("the current week is complete")
	}
	if !domain.WeekComplete(mondayNov2.AddDate(0, 0, -7), now) {
		t.Error("last week is not complete")
	}
	if !domain.WeekComplete(mondayNov2, mondayNov2.AddDate(0, 0, 7)) {
		t.Error("a week is not complete at the instant the next one starts")
	}
}

func weekShare(week time.Time, agent domain.AgentID, scope domain.Scope, readable bool, cards ...domain.WorkItemRef) domain.WorkItemWeekShare {
	return domain.WorkItemWeekShare{
		Week: week, AgentID: agent, Scope: scope, Runs: 2, OpenRuns: 1, Cards: cards,
		Proposals: domain.WorkItemProposals{
			Requested: 3, Approved: 1, Refused: 2, Abandoned: 1, Pending: 1,
			RevisedAfterRefusal: 1, RepeatedAfterRefusal: 1, Standing: 1,
		},
		Cost:         domain.WorkItemWeekCost{OnCardsMicros: 100, InheritedMicros: 20, OffCardMicros: 3},
		CostReadable: readable,
	}
}

var (
	measuresFactory = domain.Scope{Company: "acme", Area: "factory"}
	measuresCX      = domain.Scope{Company: "acme", Area: "cx"}
)

func TestRoleWeeks_foldsScopesIntoOneRowPerWeekAndAgent(t *testing.T) {
	t.Parallel()
	got := domain.RoleWeeks([]domain.WorkItemWeekShare{
		weekShare(mondayNov2, "developer", measuresFactory, true, "github:o/r#1", "github:o/r#2"),
		weekShare(mondayNov2, "developer", measuresCX, true, "github:o/r#2"),
	})
	if len(got) != 1 {
		t.Fatalf("rows = %+v, want one", got)
	}
	row := got[0]
	want := domain.WorkItemProposals{
		Requested: 6, Approved: 2, Refused: 4, Abandoned: 2, Pending: 2,
		RevisedAfterRefusal: 2, RepeatedAfterRefusal: 2, Standing: 2,
	}
	if row.Runs != 4 || row.OpenRuns != 2 || row.Proposals != want {
		t.Errorf("row = %+v, want the counts summed", row)
	}
	if row.Cards != 2 {
		t.Errorf("cards = %d, want 2: a card worked in two areas is one card", row.Cards)
	}
	if row.Cost == nil || *row.Cost != (domain.WorkItemWeekCost{OnCardsMicros: 200, InheritedMicros: 40, OffCardMicros: 6}) {
		t.Errorf("cost = %+v, want each part summed", row.Cost)
	}
}

func TestRoleWeeks_oneScopeWithoutCostRead_leavesEveryCostOutAndCountsIn(t *testing.T) {
	t.Parallel()
	got := domain.RoleWeeks([]domain.WorkItemWeekShare{
		weekShare(mondayNov2, "developer", measuresFactory, true),
		weekShare(mondayNov2, "developer", measuresCX, false),
	})
	if len(got) != 1 || got[0].Cost != nil {
		t.Fatalf("rows = %+v, want one row with no cost at all", got)
	}
	if got[0].Runs != 4 || got[0].Proposals.Requested != 6 {
		t.Errorf("row = %+v, want the counts of both scopes", got[0])
	}
}

func TestRoleWeeks_ordersByWeekThenAgent(t *testing.T) {
	t.Parallel()
	before := mondayNov2.AddDate(0, 0, -7)
	got := domain.RoleWeeks([]domain.WorkItemWeekShare{
		weekShare(mondayNov2, "triage", measuresFactory, true),
		weekShare(mondayNov2, "developer", measuresFactory, true),
		weekShare(before, "triage", measuresFactory, true),
	})
	type key struct {
		week  time.Time
		agent domain.AgentID
	}
	var order []key
	for _, r := range got {
		order = append(order, key{r.Week, r.AgentID})
	}
	want := []key{{before, "triage"}, {mondayNov2, "developer"}, {mondayNov2, "triage"}}
	if !slices.Equal(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestRoleWeeks_noShares_isAnEmptyListNotNil(t *testing.T) {
	t.Parallel()
	if got := domain.RoleWeeks(nil); got == nil || len(got) != 0 {
		t.Fatalf("RoleWeeks(nil) = %#v, want an empty list", got)
	}
}

func TestUnavailableWorkItemMeasures_namesEditedOnceAndTheBoardMeasures(t *testing.T) {
	t.Parallel()
	got := domain.UnavailableWorkItemMeasures()
	count := map[string]int{}
	reasons := map[string]string{}
	for _, u := range got {
		count[u.Measure]++
		reasons[u.Measure] = u.Reason
		if u.Detail == "" {
			t.Errorf("%s has no detail", u.Measure)
		}
	}
	want := map[string]string{"edited": "not_recorded", "laneTime": "board_only", "caughtLater": "board_only"}
	for measure, reason := range want {
		if count[measure] != 1 || reasons[measure] != reason {
			t.Errorf("%s listed %d times with reason %q, want once with %q", measure, count[measure], reasons[measure], reason)
		}
	}
	if len(got) != len(want) {
		t.Errorf("unavailable = %+v, want exactly %v", got, want)
	}
}

func TestIsStandingDecision_onlyAnApprovalTheEngineWroteForAGrant(t *testing.T) {
	t.Parallel()
	cases := []struct {
		d    domain.ApprovalDecidedPayload
		want bool
	}{
		{domain.ApprovalDecidedPayload{Approved: true, Note: domain.StandingApprovalNotePrefix + "sa_1: nightly"}, true},
		{domain.ApprovalDecidedPayload{Approved: true, Note: "looks fine"}, false},
		{domain.ApprovalDecidedPayload{Approved: false, Note: domain.StandingApprovalNotePrefix + "sa_1: x"}, false},
	}
	for _, c := range cases {
		if got := domain.IsStandingDecision(c.d); got != c.want {
			t.Errorf("IsStandingDecision(%+v) = %v, want %v", c.d, got, c.want)
		}
	}
}
