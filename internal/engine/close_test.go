package engine

import (
	"bytes"
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
A run whose closing answer is a verdict is closed by its Closer (NT-016).

The engine stores what the Closer sealed and nothing of its own: not the answer as the
model wrote it, not a headline, not an artifact. Every one of those is the judge's words,
unsalted and of whatever length the value made them.
*/

// sealingCloser seals the answer the way a judge's closer would, and records what it saw.
type sealingCloser struct {
	closing Closing
	seen    []Proposal
}

func (c *sealingCloser) Close(p Proposal) Closing {
	c.seen = append(c.seen, p)
	return c.closing
}

func TestRun_closedByACloser_storesOnlyWhatItSealed(t *testing.T) {
	t.Parallel()
	closer := &sealingCloser{closing: Closing{
		Outcome:  bytes.Repeat([]byte{0xA1}, 128),
		Judgment: bytes.Repeat([]byte{0xB2}, 64),
		Reason:   domain.RunFinishedByFinishTool,
	}}
	h := newHarness(t, Proposal{
		Done: true, Outcome: "B", Headline: "high_risk",
		Artifacts: map[string]string{"note": "risky"}, StoppedBy: "done",
	})
	start := h.start(t, generousBudget())
	start.Closer = closer

	if _, err := h.runner.Advance(context.Background(), start); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	var p domain.RunFinishedPayload
	if err := h.payloadOf(t, domain.StepRunFinished, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	outcome, err := h.content.Get(context.Background(), p.OutcomeRef)
	if err != nil || !bytes.Equal(outcome, closer.closing.Outcome) || p.OutcomeDigest != digest(outcome) {
		t.Errorf("outcome = %x (%v) digest %s, want the sealed answer and its digest", outcome, err, p.OutcomeDigest)
	}
	judgment, err := h.content.Get(context.Background(), p.JudgmentRef)
	if err != nil || !bytes.Equal(judgment, closer.closing.Judgment) || p.JudgmentDigest != digest(judgment) {
		t.Errorf("judgment = %x (%v) digest %s, want the sealed judgment and its digest", judgment, err, p.JudgmentDigest)
	}
	if p.HeadlineRef != "" || p.HeadlineDigest != "" || len(p.Artifacts) != 0 || p.StoppedBy != "" {
		t.Errorf("the closing carries the judge's other words: %+v", p)
	}
	if len(closer.seen) != 1 || closer.seen[0].Outcome != "B" || closer.seen[0].Headline != "high_risk" {
		t.Errorf("the closer saw %+v, want the whole proposal once", closer.seen)
	}
	// Two objects and no third: the answer as written and the headline are nowhere.
	if n, err := h.content.Erase(context.Background(), "run", "run-1", "count"); err != nil || n != 2 {
		t.Errorf("the run stored %d objects (%v), want the two the closer sealed", n, err)
	}
}

// A failed judgment stores no judgment and says why, in a closed code.
func TestRun_closedWithAFailure_storesNoJudgment(t *testing.T) {
	t.Parallel()
	closer := &sealingCloser{closing: Closing{
		Outcome: bytes.Repeat([]byte{0xA1}, 128),
		Reason:  domain.RunFinishedJudgeUnparseable,
	}}
	h := newHarness(t, Proposal{Done: true, Outcome: "B because"})
	start := h.start(t, generousBudget())
	start.Closer = closer

	if _, err := h.runner.Advance(context.Background(), start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	var p domain.RunFinishedPayload
	if err := h.payloadOf(t, domain.StepRunFinished, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.JudgmentRef != "" || p.JudgmentDigest != "" || p.Reason != domain.RunFinishedJudgeUnparseable {
		t.Errorf("closing = %+v, want no judgment and the failure", p)
	}
}

// An oversized answer is sealed by nobody: nothing is stored, and the code says so.
func TestRun_closedWithNothingSealed_storesNothing(t *testing.T) {
	t.Parallel()
	closer := &sealingCloser{closing: Closing{Reason: domain.RunFinishedJudgeOversized}}
	h := newHarness(t, Proposal{Done: true, Outcome: "B"})
	start := h.start(t, generousBudget())
	start.Closer = closer

	if _, err := h.runner.Advance(context.Background(), start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	var p domain.RunFinishedPayload
	if err := h.payloadOf(t, domain.StepRunFinished, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.OutcomeRef != "" || p.OutcomeDigest != "" || p.JudgmentRef != "" || p.Reason != domain.RunFinishedJudgeOversized {
		t.Errorf("closing = %+v, want nothing stored and the failure", p)
	}
	if n, _ := h.content.Erase(context.Background(), "run", "run-1", "count"); n != 0 {
		t.Errorf("the run stored %d objects, want none", n)
	}
}
