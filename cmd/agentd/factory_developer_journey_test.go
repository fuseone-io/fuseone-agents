package main

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
The factory's Developer reference stays publishable, the way the Triage
reference does (factory_triage_journey_test.go): registered as
factory/agents/README.md says, discovered by the worker, classified through
the route, and published from the Studio's form with the file as it is.

The Developer is the first role that writes to the repository's contents, so
its toolsets are the widest: repos for branches and files, pull_requests to
open one, issues for comments and labels, projects for the board.
*/

const developerReference = "../../factory/agents/developer.agent.md"

// developerToolsets is the header factory/agents/README.md gives the
// Developer registration.
const developerToolsets = "pull_requests,repos,issues,projects"

func TestFactoryDeveloper_referenceRegisteredAsDocumented_publishesFromTheStudio(t *testing.T) {
	j := newRoleJourney(t, "github-developer", developerToolsets)
	j.discover(t)
	j.classify(t)

	assertPublishesAsTheReference(t, j, readReference(t, developerReference), developerToolsets)
}

// Registered with the Reviewer's reading sets but without repos, the server
// offers nothing to write a file with, and the publish names exactly those
// tools: a Developer that cannot write is refused when it is created, not
// found out at its first card.
func TestFactoryDeveloper_toolsetsWithoutRepos_publishNamesTheMissingWriteTools(t *testing.T) {
	j := newRoleJourney(t, "github-developer", "pull_requests,issues,projects")
	j.discover(t)
	j.classify(t)

	resp := j.publish(t, readReference(t, developerReference))

	refused, ok := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse)
	if !ok || refused.Detail == nil {
		t.Fatalf("publish answered %#v; want it refused for the missing tools", resp)
	}
	for _, missing := range []string{"create_branch", "create_or_update_file", "push_files", "get_file_contents"} {
		if !strings.Contains(*refused.Detail, "github-developer."+missing) {
			t.Errorf("refusal %q does not name github-developer.%s", *refused.Detail, missing)
		}
	}
	if strings.Contains(*refused.Detail, "create_pull_request") {
		t.Errorf("refusal %q names a pull_requests tool the server does offer", *refused.Detail)
	}
}
