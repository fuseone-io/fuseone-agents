package domain_test

import (
	"reflect"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
What the console lists of a tool's rulings (NT-019).

Both rows, each with its level and where it is stored, and an owner's ruling
under a platform ruling marked as overridden. The one in force must be the one
RulingsInForce picks, or the screen and the worker would disagree about which
ruling a tool is under.
*/

var (
	listingInstallation = domain.Scope{Company: domain.Installation}
	listingCX           = domain.Scope{Company: "acme", Area: "cx"}
	listingFinance      = domain.Scope{Company: "acme", Area: "finance"}
)

func listingScopeOf(id domain.ToolID) (domain.Scope, bool) {
	switch id {
	case "cx-jira.search", "cx-jira.create", "cx-jira.close":
		return listingCX, true
	case "github.search":
		return listingInstallation, true
	}
	return domain.Scope{}, false
}

func TestRulingsOf_listsEachRowWithItsLevel_andMarksTheOverriddenOwner(t *testing.T) {
	t.Parallel()
	given := []domain.ToolClassification{
		// search: both rows; the platform's is in force, the owner's overridden.
		{Scope: listingCX, Tool: "cx-jira.search", Effect: domain.EffectRead},
		{Scope: listingInstallation, Tool: "cx-jira.search", Effect: domain.EffectWrite, Untrusted: true},
		// create: the owner's alone, in force.
		{Scope: listingCX, Tool: "cx-jira.create", Effect: domain.EffectWrite},
		// close: a sibling's row is nobody's word and is not listed.
		{Scope: listingFinance, Tool: "cx-jira.close", Effect: domain.EffectRead},
		// github: a server of the installation, one row, read before rulings had a scope.
		{Tool: "github.search", Effect: domain.EffectRead},
		// unknown: a tool scopeOf does not know.
		{Scope: listingInstallation, Tool: "gone.tool", Effect: domain.EffectRead},
	}

	want := map[domain.ToolID][]domain.ToolRuling{
		"cx-jira.search": {
			{Level: domain.RulingPlatform, Scope: listingInstallation, Effect: domain.EffectWrite, Untrusted: true, InForce: true},
			{Level: domain.RulingOwner, Scope: listingCX, Effect: domain.EffectRead, Overridden: true},
		},
		"cx-jira.create": {
			{Level: domain.RulingOwner, Scope: listingCX, Effect: domain.EffectWrite, InForce: true},
		},
		"github.search": {
			{Level: domain.RulingPlatform, Scope: listingInstallation, Effect: domain.EffectRead, InForce: true},
		},
	}
	if got := domain.RulingsOf(given, listingScopeOf); !reflect.DeepEqual(got, want) {
		t.Errorf("RulingsOf =\n%+v\nwant\n%+v", got, want)
	}
}

// Whatever order the rows arrive in, the one listed in force is the one the
// worker applies.
func TestRulingsOf_theRulingInForce_isTheOneRulingsInForcePicks(t *testing.T) {
	t.Parallel()
	owner := domain.ToolClassification{Scope: listingCX, Tool: "cx-jira.search", Effect: domain.EffectRead}
	platform := domain.ToolClassification{Scope: listingInstallation, Tool: "cx-jira.search", Effect: domain.EffectWrite}
	for name, given := range map[string][]domain.ToolClassification{
		"owner first":    {owner, platform},
		"platform first": {platform, owner},
		"owner alone":    {owner},
		"platform alone": {platform},
	} {
		picked := domain.RulingsInForce(given, listingScopeOf)
		var inForce []domain.Effect
		for _, r := range domain.RulingsOf(given, listingScopeOf)["cx-jira.search"] {
			if r.InForce {
				inForce = append(inForce, r.Effect)
			}
		}
		if len(picked) != 1 || len(inForce) != 1 || inForce[0] != picked[0].Effect {
			t.Errorf("%s: listed in force %v, RulingsInForce picked %+v", name, inForce, picked)
		}
	}
}
