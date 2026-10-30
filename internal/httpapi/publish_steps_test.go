package httpapi

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// A step's reaches is what the Gate allows while a run sits there, and the
// envelope is built from the steps rather than intersected with the pack. So a
// step naming a tool outside `tools` widened the run past the list an author
// and a reviewer read, and a typo anchored silently until a run parked on it
// (#158). A platform tool is ambient — offered by the platform whatever the
// step — and anchoring one is never what the author meant. Both are refused
// at publish, naming the step and the tool; a step within the pack publishes.
func TestPublishAgent_stepReaches_mustStayInsideThePack(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		reaches []string
		refuse  string
	}{
		{name: "a tool the pack holds", reaches: []string{"crm.lookup"}},
		{name: "a tool outside the pack", reaches: []string{"crm.lokup"}, refuse: "crm.lokup"},
		{name: "a platform tool", reaches: []string{string(domain.ToolMemoryFind)},
			refuse: string(domain.ToolMemoryFind)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pub := newPublisher()
			resp, err := publishServer(t, pub).PublishAgent(
				inArea("cx", domain.RoleAuthor),
				openapi.PublishAgentRequestObject{AgentId: "triage", Body: definition(func(d *openapi.AgentDefinition) {
					d.Tools = &[]string{"crm.lookup", string(domain.ToolMemoryFind)}
					d.Steps = &[]openapi.AgentStep{{Name: "Look up", Reaches: &tc.reaches}}
				})},
			)
			if err != nil {
				t.Fatalf("PublishAgent: %v", err)
			}

			refused, isRefusal := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse)
			if tc.refuse == "" {
				if isRefusal || len(pub.published) != 1 {
					t.Fatalf("response = %T (%d published), want the version published", resp, len(pub.published))
				}
				return
			}
			if !isRefusal || len(pub.published) != 0 {
				t.Fatalf("response = %T (%d published), want a refusal and nothing written", resp, len(pub.published))
			}
			if refused.Detail == nil || !strings.Contains(*refused.Detail, `"Look up"`) ||
				!strings.Contains(*refused.Detail, tc.refuse) {
				t.Errorf("detail = %v, want the step and %s named", refused.Detail, tc.refuse)
			}
		})
	}
}

// Keeping steps inside the pack does not catch a typo duplicated into the pack
// itself: `tools: [crm.lokup]` reaching crm.lokup published, and the agent
// stopped at its first call because the Gate never allows a tool nobody
// provides. A tool the catalogue does not know is refused at publish, and so
// is one discovered but kept off the installation's surface: it is never
// described to a model nor invoked. One on the surface the Curator has not
// ruled on still publishes — that is a server just connected — and platform
// tools are not the catalogue's.
func TestPublishAgent_toolTheCatalogueDoesNotKnow_isRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		tools  []string
		refuse string
	}{
		{name: "a known tool", tools: []string{"crm.lookup"}},
		{name: "a misspelt tool", tools: []string{"crm.lokup"}, refuse: "crm.lokup"},
		{name: "a platform tool", tools: []string{"crm.lookup", string(domain.ToolMemoryFind)}},
		{name: "a known tool not yet ruled on", tools: []string{"crm.lookup", "crm.archive"}},
		{name: "a known tool kept off the surface", tools: []string{"crm.purge"}, refuse: "crm.purge"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pub := newPublisher()
			catalogue := publishFlowTools()
			catalogue.entries = append(catalogue.entries,
				domain.ToolEntry{ID: "crm.archive", OnSurface: true},
				domain.ToolEntry{ID: "crm.purge", OnSurface: false})
			resp, err := publishServer(t, pub).WithAdministration(nil, catalogue, nil).PublishAgent(
				inArea("cx", domain.RoleAuthor),
				openapi.PublishAgentRequestObject{AgentId: "triage", Body: definition(func(d *openapi.AgentDefinition) {
					d.Tools = &tc.tools
					d.Steps = &[]openapi.AgentStep{{Name: "Look up", Reaches: &[]string{tc.tools[0]}}}
				})},
			)
			if err != nil {
				t.Fatalf("PublishAgent: %v", err)
			}

			refused, isRefusal := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse)
			if tc.refuse == "" {
				if isRefusal || len(pub.published) != 1 {
					t.Fatalf("response = %T (%v), want the version published", resp, refused.Detail)
				}
				return
			}
			if !isRefusal || len(pub.published) != 0 {
				t.Fatalf("response = %T (%d published), want a refusal and nothing written", resp, len(pub.published))
			}
			if refused.Detail == nil || !strings.Contains(*refused.Detail, tc.refuse) {
				t.Errorf("detail = %v, want %s named", refused.Detail, tc.refuse)
			}
		})
	}
}
