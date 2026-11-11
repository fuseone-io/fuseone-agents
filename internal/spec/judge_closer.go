package spec

import (
	"context"

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
	proposal.Done, proposal.Outcome = true, ""
	return proposal, nil
}

/*
verdictCloser seals a judge's answer (NT-016 § What the ledger holds).

The answer is one value of the declared shape, or a failed judgment: never trimmed, never
cut to a valid prefix, never a default. A headline, an artifact or a stopping condition is
more than the rubric allows, and none of them is stored. What is stored is salted and of
one fixed size, so neither its digest nor its length gives the value back after erasure.
*/
type verdictCloser struct {
	rubric judge.Rubric
	salt   []byte
}

func (c verdictCloser) Close(p engine.Proposal) engine.Closing {
	sealed, fits := judge.SealAnswer(c.salt, p.Outcome)
	if !fits {
		return engine.Closing{Reason: domain.RunFinishedJudgeOversized}
	}
	option, ok := c.rubric.Parse(p.Outcome)
	if !ok || p.Headline != "" || len(p.Artifacts) > 0 || p.StoppedBy != "" {
		return engine.Closing{Outcome: sealed, Reason: domain.RunFinishedJudgeUnparseable}
	}
	return engine.Closing{
		Outcome:  sealed,
		Judgment: judge.SealJudgment(c.salt, option),
		Reason:   domain.RunFinishedByFinishTool,
	}
}
