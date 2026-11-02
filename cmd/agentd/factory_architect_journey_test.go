package main

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
The factory's Architect reference stays publishable, the way the Triage and
Developer references do (factory_triage_journey_test.go): registered as
factory/agents/README.md says, discovered by the worker, classified through
the route, and published from the Studio's form with the file as it is.

The Architect reads the code to verify a plan and writes only to the issue
tracker and the board: issues for comments, the label and sub-issues,
projects for the board, and repos for its reads. The repos set also offers
writes; the reference does not declare them, and its App holds Contents:
read, so GitHub would refuse them anyway.
*/

const architectReference = "../../factory/agents/architect.agent.md"

// architectToolsets is the header factory/agents/README.md gives the
// Architect registration.
const architectToolsets = "issues,projects,repos"

func TestFactoryArchitect_referenceRegisteredAsDocumented_publishesFromTheStudio(t *testing.T) {
	j := newRoleJourney(t, "github-architect", architectToolsets)
	j.discover(t)
	j.classify(t)

	assertPublishesAsTheReference(t, j, readReference(t, architectReference), architectToolsets)
}

// Registered with Triage's sets, without repos, the server offers nothing to
// read the code with, and the publish names exactly those tools: an
// Architect that cannot verify a plan is refused when it is created, not
// found out at its first card.
func TestFactoryArchitect_toolsetsWithoutRepos_publishNamesTheMissingReadTools(t *testing.T) {
	j := newRoleJourney(t, "github-architect", triageToolsets)
	j.discover(t)
	j.classify(t)

	resp := j.publish(t, readReference(t, architectReference))

	refused, ok := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse)
	if !ok || refused.Detail == nil {
		t.Fatalf("publish answered %#v; want it refused for the missing tools", resp)
	}
	for _, missing := range []string{"get_file_contents", "search_code", "list_commits", "get_commit"} {
		if !strings.Contains(*refused.Detail, "github-architect."+missing) {
			t.Errorf("refusal %q does not name github-architect.%s", *refused.Detail, missing)
		}
	}
	if strings.Contains(*refused.Detail, "issue_write") {
		t.Errorf("refusal %q names an issues tool the server does offer", *refused.Detail)
	}
}
