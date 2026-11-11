package spec

import (
	"context"
	"encoding/json"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/judge"
)

/*
blocksPlanner sends a judge only its blocks (NT-016 § Untrusted output).

The engine builds every run's transcript from its ledger, and a judge run's opening holds
the case input. What the judge sees is the case and the judged output, each in its marked
block, and nothing else: no tools and no memory. A judge that asks for a tool has not
answered, so its proposal closes the run with no answer, which is a failed judgment.
*/
type blocksPlanner struct {
	inner engine.Planner
	turn  string
}

func (p blocksPlanner) Plan(ctx context.Context, in engine.PlanInput) (engine.Proposal, error) {
	in.Transcript = []engine.Turn{{Kind: engine.TurnInput, Text: p.turn}}
	in.Tools, in.Unavailable = nil, nil
	in.MemoryLearning = domain.MemoryLearningPolicy{}
	proposal, err := p.inner.Plan(ctx, in)
	if err != nil || proposal.Done {
		return proposal, err
	}
	// What the call cost and what it was sent stay; the tool it asked for does not.
	proposal.Tool, proposal.Args, proposal.Estimate = "", nil, domain.Consumption{}
	proposal.Done, proposal.Outcome, proposal.Verbatim = true, "", engine.Verbatim{}
	return proposal, nil
}

/*
verdictCloser seals a judge's answer (NT-016 § What the ledger holds).

It reads the answer as the provider returned it, never the adapter's normalised fields,
which trim a summary and drop an empty headline for every other run. The answer is one
finish call whose arguments hold a summary and nothing else, with no text beside it, and
the summary is one value of the declared shape. Anything else is a failed judgment: never
trimmed, never cut to a valid prefix, never a default. What is stored is salted and of one
fixed size, so neither its digest nor its length gives the value back after erasure.
*/
type verdictCloser struct {
	rubric judge.Rubric
	salt   []byte
}

func (c verdictCloser) Close(p engine.Proposal) engine.Closing {
	answer, closed := verbatimAnswer(p.Verbatim)
	sealed, fits := judge.SealAnswer(c.salt, answer)
	if !fits {
		return engine.Closing{Reason: domain.RunFinishedJudgeOversized}
	}
	option, ok := c.rubric.Parse(answer)
	if !closed || !ok {
		return engine.Closing{Outcome: sealed, Reason: domain.RunFinishedJudgeUnparseable}
	}
	return engine.Closing{
		Outcome:  sealed,
		Judgment: judge.SealJudgment(c.salt, option),
		Reason:   domain.RunFinishedByFinishTool,
	}
}

/*
verbatimAnswer is the summary of a finish call exactly as written, and whether the call
said nothing else. A key beside the summary — a headline, artifacts, a stopping condition,
even empty or of spaces — or text beside the call is more than a rubric allows. The
answer is still returned when it can be read, so it is stored as the judge wrote it.
*/
func verbatimAnswer(v engine.Verbatim) (string, bool) {
	if v.FinishArgs == nil {
		return v.Text, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(v.FinishArgs, &fields); err != nil {
		return string(v.FinishArgs), false
	}
	var summary string
	if err := json.Unmarshal(fields["summary"], &summary); err != nil {
		return string(v.FinishArgs), false
	}
	return summary, len(fields) == 1 && v.Text == ""
}
