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
