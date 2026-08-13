package compensate_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/compensate"
	"github.com/fuseone/agents/internal/domain"
)

// Compensation calls real tools on the failure path, which makes it the most
// dangerous code here. Everything in this file is about it undoing exactly
// what happened and nothing else.

type catalogue map[domain.ToolID]domain.ToolID

func (c catalogue) CompensatedBy(tool domain.ToolID) (domain.ToolID, bool) {
	undo, ok := c[tool]
	return undo, ok
}

func called(seq int64, tool string) domain.Step {
	payload, _ := json.Marshal(domain.ToolCalledPayload{
		Tool: domain.ToolID(tool), Effect: domain.EffectWrite, ArgsRef: "ref/" + tool,
	})
	return domain.Step{
		RunID: "run-1", Seq: seq, Kind: domain.StepToolCalled,
		At: time.Now(), Payload: payload,
	}
}

func returned(seq int64, tool string, failed bool) domain.Step {
	payload, _ := json.Marshal(domain.ToolReturnedPayload{
		Tool: domain.ToolID(tool), Failed: failed,
	})
	return domain.Step{
		RunID: "run-1", Seq: seq, Kind: domain.StepToolReturned,
		At: time.Now(), Payload: payload,
	}
}

func TestPlan_undoesInReverse(t *testing.T) {
	t.Parallel()

	// The order is the whole correctness of it. Undoing the charge before the
	// order it paid for leaves a refund against nothing.
	got := compensate.Plan([]domain.Step{
		called(2, "crm.order"), returned(3, "crm.order", false),
		called(4, "crm.charge"), returned(5, "crm.charge", false),
	}, catalogue{"crm.order": "crm.order.cancel", "crm.charge": "crm.charge.refund"})

	if len(got) != 2 {
		t.Fatalf("plan = %+v", got)
	}
	if got[0].Undo != "crm.charge.refund" || got[1].Undo != "crm.order.cancel" {
		t.Errorf("plan = %+v, want the charge undone first", got)
	}
}

func TestPlan_skipsACallThatNeverLanded(t *testing.T) {
	t.Parallel()

	// The tool was asked and answered with a failure, so there is nothing to
	// take back — and compensating it would perform a refund for a charge
	// that never happened.
	got := compensate.Plan([]domain.Step{
		called(2, "crm.charge"), returned(3, "crm.charge", true),
	}, catalogue{"crm.charge": "crm.charge.refund"})

	if len(got) != 0 {
		t.Errorf("plan = %+v, want nothing undone", got)
	}
}

func TestPlan_aCallStillInFlight_isNotUndone(t *testing.T) {
	t.Parallel()

	// No answer came back. Whether it landed is unknown, and undoing on a
	// guess is how a compensation becomes the damage.
	got := compensate.Plan([]domain.Step{called(2, "crm.charge")},
		catalogue{"crm.charge": "crm.charge.refund"})

	if len(got) != 0 {
		t.Errorf("plan = %+v, want nothing undone on a guess", got)
	}
}

func TestPlan_aToolWithNoCompensation_isReportedNotSkipped(t *testing.T) {
	t.Parallel()

	// A sent email is sent. The run must not report itself cleanly undone
	// when part of what it did stands — somebody has to know what is left.
	got := compensate.Plan([]domain.Step{
		called(2, "email.send"), returned(3, "email.send", false),
		called(4, "crm.charge"), returned(5, "crm.charge", false),
	}, catalogue{"crm.charge": "crm.charge.refund"})

	if len(got) != 2 {
		t.Fatalf("plan = %+v, want both accounted for", got)
	}
	if got[0].Undo != "crm.charge.refund" {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].Undo != "" || got[1].Tool != "email.send" {
		t.Errorf("second = %+v, want it named as standing", got[1])
	}
}

func TestPlan_ignoresReads(t *testing.T) {
	t.Parallel()

	// Reading changed nothing, so there is nothing to take back — and a
	// catalogue that named a compensation for a read would be describing
	// something other than a read.
	read, _ := json.Marshal(domain.ToolCalledPayload{
		Tool: "crm.lookup", Effect: domain.EffectRead,
	})
	step := domain.Step{RunID: "run-1", Seq: 2, Kind: domain.StepToolCalled, Payload: read}

	got := compensate.Plan([]domain.Step{step, returned(3, "crm.lookup", false)},
		catalogue{"crm.lookup": "crm.forget"})
	if len(got) != 0 {
		t.Errorf("plan = %+v, want reads left alone", got)
	}
}

func TestPlan_somethingAlreadyCompensated_isNotUndoneTwice(t *testing.T) {
	t.Parallel()

	compensated, _ := json.Marshal(domain.CompensatedPayload{
		Tool: "crm.charge", ForSeq: 2, Succeeded: true,
	})
	got := compensate.Plan([]domain.Step{
		called(2, "crm.charge"), returned(3, "crm.charge", false),
		{RunID: "run-1", Seq: 4, Kind: domain.StepCompensated, Payload: compensated},
	}, catalogue{"crm.charge": "crm.charge.refund"})

	// A resumed run reading the trail again must not refund twice. The record
	// of having undone it is in the same ledger as the doing.
	if len(got) != 0 {
		t.Errorf("plan = %+v, want it left alone", got)
	}
}
