package spec_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

/*
The Developer links the pull request it opens to the card it holds (#204,
slice 2b), so the issue's trail shows the Reviewer's and QA's runs on the pull
request. The instruction is one sealed line, right after the pull request is
opened, and the tool is in the Developer's tools: a reference that tells the
model to call a tool it never declared would plan every run without it, and
nobody would see why the trail stopped at the issue.
*/

// linkLine is the Developer's link instruction, on a line of its own.
const linkLine = "Right after create_pull_request succeeds, call $fuseone.work_item.link with the pull request's reference, github:fuseone-io/fuseone-agents#<pull request number>, and with nothing else."

func TestFactoryReferences_developer_linksItsPullRequestRightAfterOpeningIt(t *testing.T) {
	t.Parallel()

	data, ok := factoryReferences(t)["developer"]
	if !ok {
		t.Fatal("there is no Developer reference")
	}
	lines := strings.Split(string(data), "\n")
	if count := countLines(lines, linkLine); count != 1 {
		t.Fatalf("the Developer reference carries the link line %d times, want 1", count)
	}
	at := slices.Index(lines, linkLine)
	if claimAt := slices.Index(lines, claimLine); claimAt < 0 || at < claimAt {
		t.Errorf("the link line (line %d) comes before the claim line (line %d); a run links the card it holds", at, claimAt)
	}
	opened := slices.IndexFunc(lines, func(l string) bool { return l == "Open the pull request" })
	labelled := slices.IndexFunc(lines, func(l string) bool { return l == "- Then label the pull request factory:needs-review." })
	if opened < 0 || labelled < 0 || at < opened || at > labelled {
		t.Errorf("the link line (line %d) is not in Open the pull request (%d), before its label (%d)", at, opened, labelled)
	}

	def, err := spec.Parse("developer", data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !slices.Contains(def.Tools, domain.ToolWorkItemLink) {
		t.Errorf("the Developer's tools %v do not declare %s", def.Tools, domain.ToolWorkItemLink)
	}
}

// The link is the Developer's alone: no other role opens pull requests, and
// a role that declared the tool would be offered it on every run.
func TestFactoryReferences_onlyTheDeveloper_declaresOrCallsTheLink(t *testing.T) {
	t.Parallel()

	for role, data := range factoryReferences(t) {
		if role == "developer" {
			continue
		}
		def, err := spec.Parse(role, data)
		if err != nil {
			t.Fatalf("%s: Parse: %v", role, err)
		}
		if slices.Contains(def.Tools, domain.ToolWorkItemLink) || strings.Contains(string(data), string(domain.ToolWorkItemLink)) {
			t.Errorf("the %s reference declares or names %s", role, domain.ToolWorkItemLink)
		}
	}
}
