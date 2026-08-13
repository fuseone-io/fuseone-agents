/*
Package compensate works out what a failed run left behind (PRD SE-08).

It reads the trail and nothing else. What a run actually did is the sequence of
tool calls that came back, and any other account of it — a list kept in memory,
a summary written as it went — is a second record that disagrees with the first
one exactly when something went wrong, which is the only time this is read.

Planning is separate from performing on purpose. Compensation calls real tools
on the failure path, which makes it the most dangerous thing here, and a plan
is something a person can be shown before it runs.
*/
package compensate

import (
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// Catalogue says how a tool is undone, declared here by the consumer.
type Catalogue interface {
	CompensatedBy(tool domain.ToolID) (domain.ToolID, bool)
}

// Act is one thing the run did that has to be dealt with.
type Act struct {
	// Tool is what was called, and Seq is where. The sequence anchors the
	// compensation to the exact call it undoes, so a run that called the same
	// tool twice undoes both rather than one of them twice.
	Tool domain.ToolID
	Seq  int64
	// Undo is the tool that takes it back, empty when nothing does.
	//
	// An empty one is reported rather than dropped. A sent email is sent, and
	// a run that reported itself cleanly undone while part of what it did
	// stands would be telling somebody the opposite of what they need to know.
	Undo domain.ToolID

	// ResultRef points at what the original call returned, which is what the
	// undo is called with.
	//
	// The convention is that the undo takes what the do returned: a tool that
	// creates something answers with its identifier, and the tool that removes
	// it asks for that identifier. It is the shape almost every API that has a
	// reversal already has, and it needs no mapping language, no second schema
	// and no Curator translating field names between two tools they did not
	// write. Where the shapes genuinely do not line up, the honest answer is
	// that no compensator can be declared — not a mapping nobody can audit.
	ResultRef string
}

/*
Plan is what a run left standing, newest first.

Reverse order because that is the only order that makes sense: undoing the
order before the charge it paid for leaves a refund against nothing.

Only calls that came back, and came back without failing. A call still in
flight might have landed or might not, and undoing on a guess is how a
compensation becomes the damage it was meant to limit.
*/
func Plan(steps []domain.Step, catalogue Catalogue) []Act {
	calls := map[int64]*call{}
	var order []int64

	for _, step := range steps {
		switch step.Kind {
		case domain.StepToolCalled:
			var p domain.ToolCalledPayload
			if decode(step, &p) != nil {
				continue
			}
			calls[step.Seq] = &call{tool: p.Tool, seq: step.Seq, effect: p.Effect}
			order = append(order, step.Seq)

		case domain.StepToolReturned:
			var p domain.ToolReturnedPayload
			if decode(step, &p) != nil {
				continue
			}
			// The answer belongs to the most recent unanswered call: a run
			// that called the same tool twice must not mark the first one
			// returned by the second one's answer.
			if at := pending(calls, order); at != nil {
				at.returned, at.failed = true, p.Failed
				at.resultRef = p.ResultRef
			}

		case domain.StepCompensated:
			var p domain.CompensatedPayload
			if decode(step, &p) != nil {
				continue
			}
			// Recorded in the same ledger as the doing, so a resumed run
			// reading the trail again does not refund twice.
			if at, ok := calls[p.ForSeq]; ok && p.Succeeded {
				at.undone = true
			}
		}
	}

	var out []Act
	for i := len(order) - 1; i >= 0; i-- {
		at := calls[order[i]]
		switch {
		case at.effect == domain.EffectRead:
			// Reading changed nothing, so there is nothing to take back.
			continue
		case !at.returned || at.failed || at.undone:
			continue
		}
		undo, _ := catalogue.CompensatedBy(at.tool)
		out = append(out, Act{
			Tool: at.tool, Seq: at.seq, Undo: undo, ResultRef: at.resultRef,
		})
	}
	return out
}

// call is one tool invocation as the trail records it.
type call struct {
	tool     domain.ToolID
	seq      int64
	effect   domain.Effect
	returned bool
	failed   bool
	undone   bool
	// resultRef is what the call answered with, and what the undo is called
	// with in turn.
	resultRef string
}

// pending is the most recent call still waiting for its answer.
func pending(calls map[int64]*call, order []int64) *call {
	for i := len(order) - 1; i >= 0; i-- {
		if at := calls[order[i]]; !at.returned {
			return at
		}
	}
	return nil
}

func decode(step domain.Step, into any) error {
	if len(step.Payload) == 0 {
		return nil
	}
	return json.Unmarshal(step.Payload, into)
}

func jsonOf(v any) ([]byte, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("compensate: encode: %w", err)
	}
	return out, nil
}
