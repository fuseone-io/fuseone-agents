package domain_test

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestToolID_workItemClaimIsPlatformOwnedWithNoServer(t *testing.T) {
	t.Parallel()

	id := domain.ToolWorkItemClaim
	if id != "$fuseone.work_item.claim" {
		t.Fatalf("claim id = %q, want the id the note and the role specs name", id)
	}
	if !id.PlatformOwned() {
		t.Fatalf("%s: PlatformOwned = false, want true", id)
	}
	if server := id.Server(); server != "" {
		t.Fatalf("%s: Server = %q, want none", id, server)
	}
}

func TestToolID_coordination_isTheWorkItemClaimAlone(t *testing.T) {
	t.Parallel()

	if !domain.ToolWorkItemClaim.Coordination() {
		t.Fatalf("%s: Coordination = false, want true", domain.ToolWorkItemClaim)
	}
	// Closed by name, like PlatformOwned: being platform-owned is not enough,
	// and a near miss of the id must not borrow the exemption.
	for _, id := range []domain.ToolID{
		domain.ToolMemorySuggest, domain.ToolMemoryFind,
		domain.ToolContextRead, domain.ToolTicketAnswer,
		"$fuseone.work_item.clam", "$fuseone.work_item.claim ", "github.claim", "",
	} {
		if id.Coordination() {
			t.Errorf("%q: Coordination = true, want false", id)
		}
	}
}

func TestStepWorkItemClaimed_isARecognisedKind(t *testing.T) {
	t.Parallel()

	if !domain.StepWorkItemClaimed.Valid() {
		t.Fatalf("%s: Valid = false", domain.StepWorkItemClaimed)
	}
	if domain.StepWorkItemClaimed.Terminal() {
		t.Fatalf("%s: Terminal = true; a claim never ends a run", domain.StepWorkItemClaimed)
	}
}

func TestParseWorkItemRef(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"github issue", "github:fuseone-io/fuseone-agents#123", "github:fuseone-io/fuseone-agents#123", true},
		{"board item id", "PVTI_lADOBx", "PVTI_lADOBx", true},
		{"surrounding space is not part of the card", "  card-7 \n", "card-7", true},
		{"empty", "", "", false},
		{"only space", "   ", "", false},
		{"control character inside", "card\x1f7", "", false},
		{"newline inside", "card\n7", "", false},
		{"too long", strings.Repeat("x", domain.MaxWorkItemRefBytes+1), "", false},
		{"at the limit", strings.Repeat("x", domain.MaxWorkItemRefBytes), strings.Repeat("x", domain.MaxWorkItemRefBytes), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := domain.ParseWorkItemRef(tc.in)
			if ok != tc.ok || got != domain.WorkItemRef(tc.want) {
				t.Fatalf("ParseWorkItemRef(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}
