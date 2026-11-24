package spec

import (
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
)

// Unplaced states the engine's rule, so it is checked against the engine: a
// tool it lists is one a run is offered at no step, and a tool it leaves out
// is offered at the first step, before anything has been called.
func TestUnplaced_agreesWithTheEnvelopeTheEngineBuilds(t *testing.T) {
	t.Parallel()
	for name, s := range map[string]Spec{
		"one step reaches part of the pack": {
			Tools: []domain.ToolID{"a.read", "a.write", "b.alert"},
			Steps: []Step{{Name: "one", Reaches: []domain.ToolID{"a.read"}}},
		},
		"the last step reaches it": {
			Tools: []domain.ToolID{"a.read", "b.alert"},
			Steps: []Step{{Name: "one", Reaches: []domain.ToolID{"a.read"}}, {Name: "two", Reaches: []domain.ToolID{"b.alert"}}},
		},
		"a step that reaches nothing": {
			Tools: []domain.ToolID{"a.read", domain.ToolMemorySuggest},
			Steps: []Step{{Name: "think"}},
		},
		"no steps": {Tools: []domain.ToolID{"a.read", "b.alert"}},
	} {
		start := engine.Start{Pack: gate.NewPack(s.Tools...), Steps: envelopes(s)}
		offered := engine.EnvelopeFor(start, nil)
		var never []domain.ToolID
		for _, tool := range s.Tools {
			if !offered.Allows(tool) && !tool.PlatformOwned() {
				never = append(never, tool)
			}
		}
		if got := s.Unplaced(); !slices.Equal(got, never) {
			t.Errorf("%s: Unplaced = %v, the engine never offers %v", name, got, never)
		}
	}
}
