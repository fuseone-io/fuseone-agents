package spec

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/fuseone/agents/internal/domain"
)

// Emits is the composition contract this agent publishes when a run finishes.
//
// The YAML accepts the old compact form:
//
//	emits:
//	  - ticket.triaged
//
// and the context-carrying form:
//
//	emits:
//	  - event: incident.triaged
//	    context: incident
//	    artifacts: [triage_summary]
//
// Both become the same internal shape. The compact form remains valid because
// old definitions should not need a migration just to keep publishing the same
// static event graph.
type Emits []domain.AgentEvent

func (e *Emits) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
		return nil
	}
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("emits must be a list")
	}

	out := make(Emits, 0, len(value.Content))
	for _, node := range value.Content {
		switch node.Kind {
		case yaml.ScalarNode:
			out = append(out, domain.AgentEvent{Event: node.Value})
		case yaml.MappingNode:
			var one domain.AgentEvent
			if err := node.Decode(&one); err != nil {
				return err
			}
			out = append(out, one)
		default:
			return fmt.Errorf("emits entries must be event names or event objects")
		}
	}
	*e = out
	return nil
}

func (e Emits) MarshalYAML() (any, error) {
	out := make([]any, 0, len(e))
	for _, one := range e {
		if one.Context == "" && len(one.Artifacts) == 0 {
			out = append(out, one.Event)
			continue
		}
		out = append(out, one)
	}
	return out, nil
}

func (e *Emits) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make(Emits, 0, len(raw))
	for _, item := range raw {
		var name string
		if err := json.Unmarshal(item, &name); err == nil {
			out = append(out, domain.AgentEvent{Event: name})
			continue
		}
		var one domain.AgentEvent
		if err := json.Unmarshal(item, &one); err != nil {
			return err
		}
		out = append(out, one)
	}
	*e = out
	return nil
}

func emitProblems(emits Emits) []string {
	var problems []string
	seen := map[string]bool{}
	for _, one := range emits {
		if one.Event == "" {
			problems = append(problems, "emits entries need an event name")
			continue
		}
		if seen[one.Event] {
			problems = append(problems, fmt.Sprintf("event %q is declared twice", one.Event))
			continue
		}
		seen[one.Event] = true
	}
	return problems
}
