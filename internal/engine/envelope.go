package engine

import (
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

/*
envelopeOf is what a run may reach, given the steps it has already used.

A specification can declare steps: ordered stages, each naming the tools
reachable while a run sits in it (NT-003 §8). This narrows the frozen pack to
the stage the run has got to, and it does so without anything having to judge
whether a stage is finished — the proposal itself moves the run forward.

Forward stays open, and going back is refused. Forbidding the author's
second-favourite ordering would describe their first draft rather than their
process; letting a run that has already replied look the customer up again
would make the order decorative. So the envelope is every step from the
furthest one already used onwards.

A specification with no steps keeps its whole pack, which is how every agent
behaved before steps existed.
*/
// EnvelopeFor is envelopeOf, exported so the end-to-end suite can state the
// guarantee in the vocabulary an operator would use.
func EnvelopeFor(start Start, called []domain.ToolID) gate.Pack {
	return envelopeOf(start, called)
}

func envelopeOf(start Start, called []domain.ToolID) gate.Pack {
	if len(start.Steps) == 0 {
		return start.Pack
	}

	var reachable []domain.ToolID
	for _, step := range start.Steps[StepAt(start, called):] {
		reachable = append(reachable, step.Reaches...)
	}
	// Built from the steps rather than intersected with the pack: a tool
	// granted and then left out of every step was never placed anywhere, and
	// the safe reading of that is that nobody meant it to run.
	return gate.NewPack(reachable...)
}

func envelopeForState(start Start, state State) gate.Pack {
	pack := withoutTool(envelopeOf(start, state.Called), domain.ToolMemorySuggest)
	// A run opened from a ticket can always answer it. Answering is why that
	// run exists, and leaving it to a step's declared reach made the tool
	// something an author has to both grant and place: forgetting the second
	// finished runs with an answer nobody read, and the ledger said only that
	// the model chose to stop.
	if state.Ticket.Ref.Valid() {
		pack = gate.NewPack(append(pack.Tools(), domain.ToolTicketAnswer)...)
	}
	if start.MemoryLearning.Enabled() {
		pack = gate.NewPack(append(pack.Tools(), domain.ToolMemoryFind, domain.ToolMemorySuggest)...)
	}
	if len(state.ContextArtifacts) == 0 {
		return pack
	}
	return gate.NewPack(append(pack.Tools(), domain.ToolContextRead)...)
}

func withoutTool(pack gate.Pack, remove domain.ToolID) gate.Pack {
	tools := pack.Tools()
	out := tools[:0]
	for _, tool := range tools {
		if tool != remove {
			out = append(out, tool)
		}
	}
	return gate.NewPack(out...)
}

// closedTools is what forward movement has put behind a run: tools reachable
// in steps before the one it is at, and on the surface of this turn no more.
// Judged against envelopeForState — the surface the model actually sees —
// never the bare step union: memory learning, a ticket and context artifacts
// re-offer platform tools whatever step anchored them, and a payload calling
// one of those closed would contradict the prompt it describes. Recorded on
// the planned step so the trail can say the envelope narrowed because the
// run advanced — which otherwise reads exactly like an outage (#152).
func closedTools(start Start, state State) []domain.ToolID {
	at := StepAt(start, state.Called)
	if at == 0 {
		return nil
	}
	open := envelopeForState(start, state)
	seen := map[domain.ToolID]bool{}
	var closed []domain.ToolID
	for _, step := range start.Steps[:at] {
		for _, tool := range step.Reaches {
			if open.Allows(tool) || seen[tool] {
				continue
			}
			seen[tool] = true
			closed = append(closed, tool)
		}
	}
	return closed
}

// StepAt is the step a run has advanced to: the furthest one whose tools it
// has already used. It is what the ledger records against a proposal, so a
// correction can be anchored where the run actually was (PRD FU-13).
func StepAt(start Start, called []domain.ToolID) int {
	at := 0
	for _, tool := range called {
		// A platform tool is an ambient capability — memory, context, the
		// ticket answer — whatever step it was anchored in. Using one is not
		// evidence the run did that step's work, and advancing on it closed
		// every earlier step over a memory lookup (#152).
		if tool.PlatformOwned() {
			continue
		}
		if i, ok := stepOf(start.Steps, tool); ok && i > at {
			at = i
		}
	}
	return at
}

// StepNameAt is that step's name, or empty where a specification declared none.
func StepNameAt(start Start, called []domain.ToolID) string {
	if len(start.Steps) == 0 {
		return ""
	}
	return start.Steps[StepAt(start, called)].Name
}

/*
SpendAt is what the planning about to happen is worth spending on.

The step being *entered*, not the one just finished, and the difference is the
whole feature. A run that has used the triage step's tool is no longer
triaging: the reasoning that happens next is the reasoning that decides what to
do, and pricing it at the step behind it would give the cheap model to exactly
the decision the expensive one was configured for.

Empty where the specification declared no steps, or where the step named
nothing of its own — the agent's then, never the previous step's, which would
leak a cheap model forward into work nobody chose it for.
*/
func SpendAt(start Start, called []domain.ToolID) (model, effort string) {
	if len(start.Steps) == 0 {
		return "", ""
	}

	// Before any step's own work has been done the run is entering the first
	// step. After that it is entering the one past the furthest it has
	// reached. "Work" is a step tool, never a platform one: the engine's own
	// initial memory lookup fills Called, and letting it move the spend would
	// price reasoning still in the first step at the second's rate (#153).
	at := 0
	if stepWorked(start, called) {
		at = min(StepAt(start, called)+1, len(start.Steps)-1)
	}
	return start.Steps[at].Model, start.Steps[at].Effort
}

// stepWorked reports whether any called tool was a step's own work — a
// non-platform tool some step reaches.
func stepWorked(start Start, called []domain.ToolID) bool {
	for _, tool := range called {
		if tool.PlatformOwned() {
			continue
		}
		if _, ok := stepOf(start.Steps, tool); ok {
			return true
		}
	}
	return false
}

// stepOf reports which step first reaches a tool.
func stepOf(steps []Envelope, tool domain.ToolID) (int, bool) {
	for i, step := range steps {
		for _, t := range step.Reaches {
			if t == tool {
				return i, true
			}
		}
	}
	return 0, false
}

// stopsWhenAt is the exception the step a run has reached declared, or nothing
// where it declared none.
func stopsWhenAt(start Start, at int) string {
	if at < 0 || at >= len(start.Steps) {
		return ""
	}
	return start.Steps[at].StopsWhen
}
