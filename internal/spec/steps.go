package spec

import "github.com/fuseone/agents/internal/domain"

// Step is a stage of the process: an envelope with a gate at its exit.
//
// NT-003 §8 settled the shape by writing one real agent's steps out by hand,
// and three things fell out of it.
//
// A step is not a tool call — summarising reaches nothing at all, and a model
// where steps are tools cannot represent the simplest agent this repository
// has. The exception belongs to the step rather than to the agent, which is
// what lets a correction be localised to where it went wrong (PRD FU-13).
// And the order is between steps only: inside one the loop stays free, because
// forbidding the author's second-favourite ordering would describe their first
// draft rather than their process.
type Step struct {
	Name string `yaml:"name"`

	// Reaches is what the Gate will allow while the run is in this step. Empty
	// is meaningful: a step that calls nothing is the agent thinking.
	Reaches []domain.ToolID `yaml:"reaches,omitempty"`

	// StopsWhen is the exception, in the author's own words: "não encontrar o
	// cliente". Nothing judges it yet — who decides a step is over is a
	// separate decision, and NT-003 leaves it open on purpose.
	StopsWhen string `yaml:"stops_when,omitempty"`
}

/*
EnvelopeAt reports what a run may reach while it sits at a step.

With no steps declared there is one envelope holding the whole pack, which is
exactly today's behaviour and is what lets this land without anybody
republishing anything.

Past the last step a run has finished and reaches nothing. Falling back to the
pack there would quietly make the end of a run the loosest part of it.
*/
func (s Spec) EnvelopeAt(step int) []domain.ToolID {
	if len(s.Steps) == 0 {
		return s.Tools
	}
	if step < 0 || step >= len(s.Steps) {
		return nil
	}
	return s.Steps[step].Reaches
}
