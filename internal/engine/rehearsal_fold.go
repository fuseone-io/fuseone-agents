package engine

import "github.com/fuseone/agents/internal/domain"

// worldWait is the world_awaited step a rehearsal stands at.
type worldWait struct {
	atSeq, callSeq int64
	tool           domain.ToolID
}

/*
foldRehearsal reads the steps of a live rehearsal (NT-018 phase 4).

A wait parks the run for its author. An answer is taken only when it names the wait the
run stands at: an answer about another wait is a question the run has moved past, and
reopens nothing. The author's decision on a held action answers the Gate's request as a
person's would; that it is a different kind is what keeps it out of every count of real
approvals.
*/
func (s *State) foldRehearsal(step domain.Step) error {
	switch step.Kind {
	case domain.StepWorldAwaited:
		var p domain.WorldAwaitedPayload
		if err := decode(step, &p); err != nil {
			return err
		}
		s.waiting = &worldWait{atSeq: step.Seq, callSeq: p.CallSeq, tool: p.Tool}
		s.Phase = PhaseAwaitingWorld
	case domain.StepWorldSupplied:
		var p domain.WorldSuppliedPayload
		if err := decode(step, &p); err != nil {
			return err
		}
		if s.waiting == nil || p.AtSeq != s.waiting.atSeq {
			return nil
		}
		s.supplied = &p
		s.waiting = nil
		s.Phase = PhaseAwaitingTool
	case domain.StepRehearsalDecided:
		var p domain.RehearsalDecidedPayload
		if err := decode(step, &p); err != nil {
			return err
		}
		s.decide(domain.ApprovalDecidedPayload{Approved: p.Approved, By: p.By, AtSeq: p.AtSeq})
	}
	return nil
}
