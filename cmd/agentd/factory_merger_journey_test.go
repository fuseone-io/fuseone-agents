package main

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
The factory's Merger reference stays publishable (#199), the way the other
roles' do, with one difference: it reaches no hosted GitHub server. Its only
server is factory-merger, on the worker's loopback, and its only read is
list_ready_prs, whose result is numbers, commit SHAs, item ids and reasons.

That is what lets it merge. The publish refuses a definition in which
untrusted data can reach a tool that is not reversible, and merge_if_ready is
destructive: classified as factory/agents/README.md says, list_ready_prs's
result is not untrusted, so nothing the Merger reads taints the merge. Ruled
untrusted, the same reference is refused, naming merge_if_ready.
*/

const mergerReference = "../../factory/agents/merger.agent.md"

// newMergerJourney registers factory-merger as the setup texts say, and no
// GitHub server: the Merger has none.
func newMergerJourney(t *testing.T) *factoryJourney {
	t.Helper()
	j := newInstallation(t, "factory-merger")
	j.registerBoard(t, "merger")
	return j
}

// mergerRuling is the classification factory/agents/README.md gives the
// Merger's tools; listUntrusted overrides the one ruling the merge depends on.
func mergerRuling(listUntrusted bool) func(openapi.Tool) *openapi.ClassifyToolJSONRequestBody {
	return func(tool openapi.Tool) *openapi.ClassifyToolJSONRequestBody {
		name, ours := strings.CutPrefix(tool.ToolId, "factory-merger.")
		switch {
		case !ours:
			return nil
		case name == "list_ready_prs":
			return &openapi.ClassifyToolJSONRequestBody{
				Effect: openapi.EffectRead, Untrusted: ptrTo(listUntrusted), Digest: tool.Digest,
			}
		case name == "merge_if_ready":
			return &openapi.ClassifyToolJSONRequestBody{
				Effect: openapi.EffectDestructive, Untrusted: ptrTo(false), Digest: tool.Digest,
			}
		default:
			return &openapi.ClassifyToolJSONRequestBody{
				Effect: openapi.EffectWrite, Untrusted: ptrTo(false), Digest: tool.Digest,
			}
		}
	}
}

func TestFactoryMerger_referenceRegisteredAsDocumented_publishesFromTheStudio(t *testing.T) {
	j := newMergerJourney(t)
	j.discover(t)
	j.classifyWith(t, mergerRuling(false))

	assertPublishedIsTheReference(t, j, readReference(t, mergerReference))

	// No hosted GitHub server was registered, so nothing reached one.
	j.hosted.mu.Lock()
	defer j.hosted.mu.Unlock()
	if served := len(j.hosted.toolsets); served != 0 {
		t.Errorf("the hosted GitHub served %d requests; the Merger reaches no hosted server", served)
	}
}

// The anchor: the publish does judge the flow. With list_ready_prs ruled
// untrusted, the Merger's read reaches merge_if_ready and the same reference
// is refused, so the test above passes because of the ruling, not because
// the check never ran.
func TestFactoryMerger_listReadyPRsRuledUntrusted_publishIsRefusedNamingTheMerge(t *testing.T) {
	j := newMergerJourney(t)
	j.discover(t)
	j.classifyWith(t, mergerRuling(true))

	resp := j.publish(t, readReference(t, mergerReference))

	refused, ok := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse)
	if !ok || refused.Detail == nil {
		t.Fatalf("publish answered %#v; want it refused for the untrusted flow", resp)
	}
	if !strings.Contains(*refused.Detail, "factory-merger.merge_if_ready") {
		t.Errorf("refusal %q does not name factory-merger.merge_if_ready", *refused.Detail)
	}
}
