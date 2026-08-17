package httpapi

import (
	"context"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

/*
The half of a definition the summary leaves out.

A listing shows twenty agents and none of their processes, for the same reason
it shows none of their prose: twenty bodies of text nobody asked to read. The
stages are read one version at a time, beside the instructions they belong to.
*/
// Definitions reads the half of a specification the summary leaves out,
// declared here by the consumer.
//
// Both halves in one answer, because they are dropped for the same reason:
// what a read does not return, an editor cannot put back, and publishing
// again deletes it. Two calls would be two chances to forget one.
type Definitions interface {
	Declared(ctx context.Context, agent domain.AgentID, version domain.VersionID) (steps []spec.Step, emits spec.Emits, err error)
}

// WithDefinitions wires reading a published version's declared stages.
func (s *Server) WithDefinitions(definitions Definitions) *Server {
	s.definitions = definitions
	return s
}

func eventsFrom(declared spec.Emits) []openapi.AgentEvent {
	out := make([]openapi.AgentEvent, 0, len(declared))
	for _, event := range declared {
		one := openapi.AgentEvent{Event: event.Event}
		if event.Context != "" {
			one.Context = ptr(event.Context)
		}
		if len(event.Artifacts) > 0 {
			artifacts := append([]string(nil), event.Artifacts...)
			one.Artifacts = &artifacts
		}
		out = append(out, one)
	}
	return out
}

func stepsFrom(declared []spec.Step) []openapi.AgentStep {
	out := make([]openapi.AgentStep, 0, len(declared))
	for _, step := range declared {
		one := openapi.AgentStep{Name: step.Name}
		if len(step.Reaches) > 0 {
			reaches := make([]string, 0, len(step.Reaches))
			for _, tool := range step.Reaches {
				reaches = append(reaches, string(tool))
			}
			one.Reaches = &reaches
		}
		one.StopsWhen = someString(step.StopsWhen)
		one.Model = someString(step.Model)
		one.Effort = someString(step.Effort)
		out = append(out, one)
	}
	return out
}
