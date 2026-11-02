package domain_test

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
The trail of one card (#204, slice 1): the runs that claimed it and, per role,
what they asked people and what they cost. The stores read the facts; the
fold and the cursor are the same pure functions for both.
*/

var trailT0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func trailRun(id domain.RunID, agent domain.AgentID, claimedMin int) domain.WorkItemRun {
	return domain.WorkItemRun{
		RunID: id, AgentID: agent, Link: domain.WorkItemLinkClaim,
		ClaimedAt: trailT0.Add(time.Duration(claimedMin) * time.Minute),
	}
}

func TestWorkItemRoles_foldsRunsOfOneAgentIntoOneRow(t *testing.T) {
	t.Parallel()

	ended := trailT0.Add(3 * time.Hour)
	facts := []domain.WorkItemRunFacts{
		{Run: trailRun("run-b", "developer", 30), CostMicros: 700, CostReadable: true,
			Approvals: domain.WorkItemApprovals{Requested: 2, Approved: 1, Refused: 1}},
		{Run: trailRun("run-a", "triage", 0), CostMicros: 100, CostReadable: true,
			Approvals: domain.WorkItemApprovals{Requested: 1, Approved: 1}},
		{Run: func() domain.WorkItemRun {
			r := trailRun("run-c", "developer", 10)
			r.EndedAt = ended
			return r
		}(), CostMicros: 300, CostReadable: true,
			Approvals: domain.WorkItemApprovals{Abandoned: 1}},
	}

	roles := domain.WorkItemRoles(facts)

	if len(roles) != 2 || roles[0].AgentID != "triage" || roles[1].AgentID != "developer" {
		t.Fatalf("roles = %+v; want triage then developer, in the order each first claimed", roles)
	}
	dev := roles[1]
	want := domain.WorkItemApprovals{Requested: 2, Approved: 1, Refused: 1, Abandoned: 1}
	if dev.Runs != 2 || dev.Approvals != want {
		t.Errorf("developer = %d runs, %+v; want 2 runs, %+v", dev.Runs, dev.Approvals, want)
	}
	if !dev.FirstClaimedAt.Equal(trailT0.Add(10*time.Minute)) || !dev.LastEndedAt.Equal(ended) {
		t.Errorf("developer first claimed %v, last ended %v", dev.FirstClaimedAt, dev.LastEndedAt)
	}
	if dev.CostMicros == nil || *dev.CostMicros != 1000 {
		t.Errorf("developer cost = %v, want 1000", dev.CostMicros)
	}
}

// A role's cost is the whole of its runs or nothing: a sum that left out the
// runs the caller may not cost would be a wrong number, not a smaller one.
func TestWorkItemRoles_aRunWhoseCostIsNotReadable_omitsTheRolesCostNotZeroes(t *testing.T) {
	t.Parallel()

	roles := domain.WorkItemRoles([]domain.WorkItemRunFacts{
		{Run: trailRun("run-a", "triage", 0), CostMicros: 100, CostReadable: true},
		{Run: trailRun("run-b", "triage", 1), CostMicros: 200, CostReadable: false},
		{Run: trailRun("run-c", "developer", 2), CostMicros: 0, CostReadable: false},
	})

	for _, role := range roles {
		if role.CostMicros != nil {
			t.Errorf("%s cost = %d; want it absent", role.AgentID, *role.CostMicros)
		}
	}
}

func TestWorkItemRoles_noRunHasEnded_leavesLastEndedZero(t *testing.T) {
	t.Parallel()

	roles := domain.WorkItemRoles([]domain.WorkItemRunFacts{{Run: trailRun("run-a", "triage", 0)}})
	if len(roles) != 1 || !roles[0].LastEndedAt.IsZero() {
		t.Fatalf("roles = %+v; want one row with no end", roles)
	}
}

func TestWorkItemCursor_roundTripsAPosition(t *testing.T) {
	t.Parallel()

	at := domain.WorkItemCursorAt(trailRun("run-b", "triage", 5))
	got, err := domain.DecodeWorkItemCursor(at.Encode())
	if err != nil || got == nil || got.RunID != "run-b" || !got.ClaimedAt.Equal(at.ClaimedAt) {
		t.Fatalf("decoded = %+v, %v; want the position back", got, err)
	}
}

func TestWorkItemCursor_noToken_startsAtTheBeginning(t *testing.T) {
	t.Parallel()

	got, err := domain.DecodeWorkItemCursor("")
	if got != nil || err != nil {
		t.Fatalf("decode(\"\") = %+v, %v; want the first page", got, err)
	}
}

// A cursor carries a position and nothing else. A token that is not one —
// garbage, a half, or one that smuggles a field beside the position — is
// refused rather than read as the first page or trusted.
func TestDecodeWorkItemCursor_malformedOrCarryingMore_refused(t *testing.T) {
	t.Parallel()

	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	for name, token := range map[string]string{
		"not base64":        "%%%",
		"not json":          enc("nope"),
		"no run":            enc(`{"t":"2026-10-01T09:00:00Z"}`),
		"no instant":        enc(`{"r":"run-a"}`),
		"carries a filter":  enc(`{"t":"2026-10-01T09:00:00Z","r":"run-a","scopes":[{"company":"*"}]}`),
		"trailing data":     enc(`{"t":"2026-10-01T09:00:00Z","r":"run-a"}{}`),
		"padded base64 too": base64.URLEncoding.EncodeToString([]byte(`{"t":"2026-10-01T09:00:00Z","r":"run-a"}`)) + "!",
	} {
		if got, err := domain.DecodeWorkItemCursor(token); !errors.Is(err, domain.ErrWorkItemCursorInvalid) {
			t.Errorf("%s: decode = %+v, %v; want ErrWorkItemCursorInvalid", name, got, err)
		}
	}
}

// The trail reads oldest claim first and breaks a shared instant by run id,
// so a page boundary inside one instant neither skips nor repeats a run.
func TestWorkItemCursor_precedes_ordersByClaimThenRunID(t *testing.T) {
	t.Parallel()

	c := domain.WorkItemCursorAt(trailRun("run-b", "triage", 5))
	cases := []struct {
		run  domain.WorkItemRun
		want bool
	}{
		{trailRun("run-a", "triage", 5), false},
		{trailRun("run-b", "triage", 5), false},
		{trailRun("run-c", "triage", 5), true},
		{trailRun("run-a", "triage", 6), true},
		{trailRun("run-z", "triage", 4), false},
	}
	for _, tc := range cases {
		if got := c.Precedes(tc.run); got != tc.want {
			t.Errorf("Precedes(%s at %v) = %v, want %v", tc.run.RunID, tc.run.ClaimedAt, got, tc.want)
		}
	}
	var none *domain.WorkItemCursor
	if !none.Precedes(trailRun("run-a", "triage", 0)) {
		t.Error("no cursor precedes every run")
	}
}

func TestWorkItemPage_sizeIsBoundedWithADefault(t *testing.T) {
	t.Parallel()

	for limit, want := range map[int]int{0: 50, -1: 50, 7: 7, 200: 200, 201: 200} {
		if got := (domain.WorkItemPage{Limit: limit}).Size(); got != want {
			t.Errorf("Size(%d) = %d, want %d", limit, got, want)
		}
	}
}
