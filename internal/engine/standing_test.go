package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

type fakeStanding struct {
	grant  StandingGrant
	covers bool
	err    error
	claims int
}

func (f *fakeStanding) Claim(
	context.Context, domain.ToolID, domain.AgentID, domain.Scope, domain.RunID, time.Time,
) (StandingGrant, bool, error) {
	f.claims++
	return f.grant, f.covers, f.err
}

/*
A standing grant is SE-06's one exception made durable: the park still
happens and is still released by a human decision — the decided step carries
the grant owner's name and the grant's id — so the fold, the projection, the
transcript and the card closer never learn the human was asleep.
*/
func TestAdvance_aStandingGrantCoversTheParkedWrite_decidesWithTheOwnersName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.note", Args: []byte(`{"text":"hi"}`)})
	port := &fakeStanding{
		grant:  StandingGrant{ID: "SG-7", By: "usr_ana", Reason: "night shift"},
		covers: true,
	}
	h.runner.deps.Standing = port

	st, err := h.runner.Advance(ctx, h.start(t, generousBudget()))
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if st.Phase != PhaseRunning {
		t.Fatalf("Phase = %v, want running after the standing decision", st.Phase)
	}

	step, err := h.stepOf(t, domain.StepApprovalDecided)
	if err != nil {
		t.Fatalf("no decided step: %v", err)
	}
	var decided domain.ApprovalDecidedPayload
	if err := json.Unmarshal(step.Payload, &decided); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !decided.Approved || decided.By != "usr_ana" ||
		!strings.Contains(decided.Note, "SG-7") {
		t.Fatalf("decided = %+v, want the grant owner's name and the grant id", decided)
	}

	// The next turn executes exactly as a hand-approved run would.
	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err != nil {
		t.Fatalf("second Advance: %v", err)
	}
	if len(h.tools.invocations) != 1 {
		t.Fatalf("invocations = %v, want the approved call", h.tools.invocations)
	}
}

// No coverage, port error, or no port at all: the park stands and a human
// decides. The grant path fails closed in every direction.
func TestAdvance_withoutACoveringGrant_theParkStands(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	for name, port := range map[string]*fakeStanding{
		"no coverage": {covers: false},
		"port error":  {err: errors.New("store down")},
		"nil port":    nil,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, Proposal{Tool: "crm.note", Args: []byte(`{"text":"hi"}`)})
			if port != nil {
				h.runner.deps.Standing = port
			}
			st, err := h.runner.Advance(ctx, h.start(t, generousBudget()))
			if err != nil {
				t.Fatalf("Advance: %v", err)
			}
			if st.Phase != PhaseAwaitingApproval {
				t.Fatalf("Phase = %v, want awaiting approval", st.Phase)
			}
			if _, err := h.stepOf(t, domain.StepApprovalDecided); err == nil {
				t.Fatal("a decision was appended without a covering grant")
			}
			if len(h.tools.invocations) != 0 {
				t.Fatal("the tool ran while awaiting approval")
			}
		})
	}
}
