package memory_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/contextshare"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/memory"
)

/*
Evidence has to survive the layers between the engine and the tool that seals it.

The engine asks the outermost layer whether it can produce approval evidence,
and these two wrap every tool a worker offers. Neither answering meant the
question was never asked: the approval request carried no evidence, and the
decision was refused later for evidence that nothing had been asked to make.
*/
func TestApprovalEvidence_reachesTheToolThroughEveryLayer(t *testing.T) {
	t.Parallel()
	sealed := domain.ApprovalEvidence{
		Kind: "ticket_answer", Ticket: domain.TicketRef{Key: "ticket-1", Revision: 1},
		Ref: "content://answer", Digest: "sha256:answer",
	}
	base := &sealingTools{evidence: sealed}
	layered := memory.NewLayer(
		contextshare.New(base, base, nil), contextshare.New(base, base, nil), nil, nil,
	)

	provider, ok := any(layered).(engine.ApprovalEvidencer)
	if !ok {
		t.Fatal("the outermost layer cannot be asked for approval evidence")
	}
	got, err := provider.ApprovalEvidence(context.Background(), engine.Call{Tool: "gravitee.one.accept"})
	if err != nil || got != sealed {
		t.Fatalf("ApprovalEvidence = (%+v, %v), want what the tool sealed", got, err)
	}
}

type sealingTools struct {
	evidence domain.ApprovalEvidence
}

func (s *sealingTools) Reserve(context.Context, engine.Call) error { return nil }

func (s *sealingTools) Invoke(context.Context, engine.Call) (engine.ToolResult, error) {
	return engine.ToolResult{}, nil
}

func (s *sealingTools) Effect(domain.ToolID) (domain.Effect, bool) {
	return domain.EffectWrite, true
}

func (s *sealingTools) Dedupe(domain.ToolID) (domain.ToolDedupe, bool) {
	return domain.ToolDedupe{}, false
}

func (s *sealingTools) ApprovalEvidence(
	context.Context, engine.Call,
) (domain.ApprovalEvidence, error) {
	return s.evidence, nil
}
