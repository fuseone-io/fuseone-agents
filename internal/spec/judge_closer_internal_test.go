package spec

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

// recordingPlanner records what the judge's planner was handed and answers as told.
type recordingPlanner struct {
	got    engine.PlanInput
	answer engine.Proposal
}

func (p *recordingPlanner) Plan(_ context.Context, in engine.PlanInput) (engine.Proposal, error) {
	p.got = in
	return p.answer, nil
}

/*
The judge is handed its blocks and nothing of the run (NT-016 § Untrusted output): no
transcript the engine built, no tools, no memory.
*/
func TestBlocksPlanner_handsTheJudgeOnlyItsBlocks(t *testing.T) {
	t.Parallel()
	inner := &recordingPlanner{answer: engine.Proposal{Done: true, Outcome: "A"}}
	planner := blocksPlanner{inner: inner, turn: "<<<blocks>>>"}

	if _, err := planner.Plan(context.Background(), engine.PlanInput{
		Transcript:     []engine.Turn{{Kind: engine.TurnInput, Text: "the run's own"}, {Kind: engine.TurnToolUse}},
		Tools:          []domain.ToolID{"crm.lookup"},
		Unavailable:    []domain.ToolID{"crm.refund"},
		MemoryLearning: domain.MemoryLearningPolicy{Mode: domain.MemoryLearningReview},
	}); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	got := inner.got
	if len(got.Transcript) != 1 || got.Transcript[0].Text != "<<<blocks>>>" || got.Transcript[0].Kind != engine.TurnInput {
		t.Errorf("transcript = %+v, want the blocks alone", got.Transcript)
	}
	if len(got.Tools) != 0 || len(got.Unavailable) != 0 || got.MemoryLearning != (domain.MemoryLearningPolicy{}) {
		t.Errorf("handed tools %v, unavailable %v, memory %+v; want none", got.Tools, got.Unavailable, got.MemoryLearning)
	}
}

// A judge that asks for a tool has not answered: the run closes with no answer, which the
// closer seals as a failed judgment, and the call's cost stays on the record.
func TestBlocksPlanner_aToolProposal_closesWithNoAnswer(t *testing.T) {
	t.Parallel()
	cost := domain.Cost{InputTokens: 10, Micros: 40}
	inner := &recordingPlanner{answer: engine.Proposal{Tool: "crm.lookup", Args: []byte(`{}`), Cost: cost}}

	got, err := blocksPlanner{inner: inner, turn: "t"}.Plan(context.Background(), engine.PlanInput{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !got.Done || got.Outcome != "" || got.Tool != "" || got.Args != nil || got.Cost != cost {
		t.Errorf("proposal = %+v, want done with no answer, no tool, and the cost kept", got)
	}
}
