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
		// The canonical forms the factory uses.
		{"github issue", "github:fuseone-io/fuseone-agents#123", "github:fuseone-io/fuseone-agents#123", true},
		{"github repo with dots and underscores", "github:a/b.c_d-e#7", "github:a/b.c_d-e#7", true},
		{"github owner and repo fold case", "github:FuseOne-IO/Fuseone-Agents#123", "github:fuseone-io/fuseone-agents#123", true},
		{"board item id", "ghproject:PVTI_lADOBx-9", "ghproject:PVTI_lADOBx-9", true},
		{"surrounding space is not part of the card", "  github:a/b#1 \n", "github:a/b#1", true},
		// Everything else is not an identifier.
		{"empty", "", "", false},
		{"only space", "   ", "", false},
		{"zero-width space inside", "github:fuseone-io/fuseone-agents#12\u200B3", "", false},
		{"near duplicate with an invisible character", "github:fuseone-io\u2060/fuseone-agents#123", "", false},
		{"bidi override", "github:fuseone-io/\u202Efuseone-agents#123", "", false},
		{"email address", "ana.souza@example.com", "", false},
		{"free text", "fix the login page", "", false},
		{"bare board id without its prefix", "PVTI_lADOBx", "", false},
		{"unknown scheme", "jira:ABC-12", "", false},
		{"github without a number", "github:fuseone-io/fuseone-agents", "", false},
		{"github number with letters", "github:fuseone-io/fuseone-agents#12a", "", false},
		{"github number zero", "github:fuseone-io/fuseone-agents#0", "", false},
		{"github owner with a dot", "github:fuse.one/agents#1", "", false},
		{"github owner starting with a hyphen", "github:-fuse/agents#1", "", false},
		{"github repo that is a dot path", "github:fuse/..#1", "", false},
		{"github with a path", "github:fuse/agents/issues#1", "", false},
		{"board id with a space", "ghproject:PVTI lADOBx", "", false},
		{"board id too long", "ghproject:" + strings.Repeat("x", domain.MaxWorkItemRefBytes), "", false},
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
