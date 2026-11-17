// Package draft is an agent's definition while it is being written in the
// workbench, and the typed operations that change it (NT-018 1b).
//
// A draft is the whole definition plus its instructions as blocks. It changes
// only through operations, each on one field, and an operation is accepted
// only if the draft it leaves is one publication would not refuse for being
// wrong: Validator.Validate renders it, parses it back and runs the checks a
// publication runs. What is only unfinished is listed, never refused.
//
// Pure apart from the installation's checks, which come in through
// publishable.Checker: no storage, no HTTP.
package draft

import (
	"slices"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

// Draft is a definition being written.
type Draft struct {
	// Spec is every field but the instructions, which Blocks holds.
	Spec   spec.Spec
	Blocks []blocks.Block
}

// New is the draft the human step at creation leaves: an identifier, a name
// and a scope, and nothing else yet.
func New(id domain.AgentID, name string, scope domain.Scope) Draft {
	return Draft{Spec: spec.Spec{
		ID: id, Name: name, Company: scope.Company, Area: scope.Area,
		MemoryLearning: domain.MemoryLearningPolicy{Mode: domain.MemoryLearningOff},
	}}
}

// FromSpec opens a published definition as a draft. Its instructions become
// blocks that keep their raw bytes, so the untouched draft renders the same
// version.
func FromSpec(s spec.Spec) Draft {
	d := Draft{Spec: s, Blocks: blocks.Parse(s.Instructions)}
	d.Spec.Instructions = ""
	d.Spec.Version, d.Spec.Digest, d.Spec.Source = "", "", ""
	return d
}

// Definition is the draft as a specification, its blocks rendered.
func (d Draft) Definition() spec.Spec {
	s := d.clone().Spec
	s.Instructions = blocks.Render(d.Blocks)
	return s
}

// clone is a copy that shares no slice with the original, nested ones
// included, so an operation on it never changes the draft it was applied to.
func (d Draft) clone() Draft {
	out := d
	s := &out.Spec
	s.Tools = slices.Clone(s.Tools)
	s.Triggers = cloneTriggers(s.Triggers)
	s.Emits = cloneEmits(s.Emits)
	s.Steps = cloneSteps(s.Steps)
	s.Approvals.Notify = slices.Clone(s.Approvals.Notify)
	out.Blocks = slices.Clone(d.Blocks)
	return out
}

// The copies below are what an operation stores too: a payload kept by
// reference would let its caller change a draft after it was judged.

func cloneStep(step spec.Step) spec.Step {
	step.Reaches = slices.Clone(step.Reaches)
	return step
}

func cloneSteps(steps []spec.Step) []spec.Step {
	out := slices.Clone(steps)
	for i := range out {
		out[i] = cloneStep(out[i])
	}
	return out
}

func cloneTriggers(triggers []spec.Trigger) []spec.Trigger {
	out := slices.Clone(triggers)
	for i := range out {
		out[i].Events = slices.Clone(out[i].Events)
	}
	return out
}

func cloneEmits(emits spec.Emits) spec.Emits {
	out := slices.Clone(emits)
	for i := range out {
		out[i].Artifacts = slices.Clone(out[i].Artifacts)
	}
	return out
}
