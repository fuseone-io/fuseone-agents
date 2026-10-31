package spec_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

// The factory's role references are copied into the console by hand (v1), so
// nothing at run time reads them. These tests are what stops one drifting
// into a file the console would refuse, or into a role that has lost the rule
// its README promises it carries (factory/agents/README.md).

const factoryAgentsDir = "../../factory/agents"

// claimLine is the exact sentence every role's instructions carry on a line
// of its own (factory/agents/README.md § The claim line).
const claimLine = "First act after choosing a card: call $fuseone.work_item.claim with the card's reference before reading the issue or writing anything."

func factoryReferences(t *testing.T) map[string][]byte {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(factoryAgentsDir, "*.agent.md"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	refs := make(map[string][]byte, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		refs[strings.TrimSuffix(filepath.Base(path), ".agent.md")] = data
	}
	return refs
}

func TestFactoryReferences_roleFiles_areExactlyTriageReviewerAndQA(t *testing.T) {
	t.Parallel()

	var roles []string
	for role := range factoryReferences(t) {
		roles = append(roles, role)
	}
	slices.Sort(roles)
	// A new role arrives with its issue and its review; this list moves with it.
	if want := []string{"qa", "reviewer", "triage"}; !slices.Equal(roles, want) {
		t.Errorf("role references = %v, want %v", roles, want)
	}
}

func TestFactoryReferences_eachFile_parsesAndSurvivesARoundTrip(t *testing.T) {
	t.Parallel()

	for role, data := range factoryReferences(t) {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			first, err := spec.Parse(role, data)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			// The console renders what it publishes, so a reference that does
			// not survive rendering is not the agent a person reviewed here.
			rendered, err := spec.Render(first)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			again, err := spec.Parse(role, rendered)
			if err != nil {
				t.Fatalf("Parse rendered: %v", err)
			}
			// Version is the digest of the bytes, and the file is laid out
			// for people (flow-style triggers) rather than as Render writes
			// it; every field the definition declares must come back.
			first.Version, again.Version = "", ""
			if !reflect.DeepEqual(again, first) {
				t.Errorf("round trip changed the definition:\n got %+v\nwant %+v", again, first)
			}
		})
	}
}

func TestFactoryReferences_eachFile_carriesTheClaimLineOnce(t *testing.T) {
	t.Parallel()

	for role, data := range factoryReferences(t) {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			// A line of its own, compared whole: "Never call
			// $fuseone.work_item.claim" also mentions the tool.
			count := 0
			for _, line := range strings.Split(string(data), "\n") {
				if line == claimLine {
					count++
				}
			}
			if count != 1 {
				t.Errorf("claim line appears %d times, want 1", count)
			}
		})
	}
}

func TestFactoryReferences_eachFile_forbidsStandingApprovals(t *testing.T) {
	t.Parallel()

	for role, data := range factoryReferences(t) {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			s, err := spec.Parse(role, data)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			// A grant given ahead of time would release a factory write with
			// no person behind it (#192).
			if s.StandingApprovals != domain.StandingForbidden {
				t.Errorf("standing_approvals = %q, want forbidden", s.StandingApprovals)
			}
		})
	}
}
