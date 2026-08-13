package flow_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/flow"
)

/*
"Does personal data reach an external sending tool?" answered without running
anything (PRD SE-07). The Gate answers it per call at runtime; this answers it
while somebody can still change the specification.
*/

type catalogue map[domain.ToolID]struct {
	effect    domain.Effect
	untrusted bool
}

func (c catalogue) Effect(t domain.ToolID) (domain.Effect, bool) {
	entry, ok := c[t]
	return entry.effect, ok
}

func (c catalogue) Untrusted(t domain.ToolID) bool { return c[t].untrusted }

var known = catalogue{
	"crm.lookup": {domain.EffectRead, true},
	"kb.search":  {domain.EffectRead, false},
	"crm.reply":  {domain.EffectWrite, false},
	"pay.refund": {domain.EffectFinancial, false},
}

func TestCheck_aReadFromOutsideAndAWrite_isAPath(t *testing.T) {
	t.Parallel()

	// The question the requirement asks, in its simplest form. With no steps
	// the whole pack is one envelope and the model chooses the order, so this
	// holds whichever way the author listed them.
	got := flow.Check([]domain.ToolID{"crm.reply", "crm.lookup"}, nil, known)

	if len(got.Paths) != 1 {
		t.Fatalf("paths = %+v, want one", got.Paths)
	}
	if got.Paths[0].From != "crm.lookup" || got.Paths[0].To != "crm.reply" {
		t.Errorf("path = %s, want the lookup reaching the reply", got.Paths[0])
	}
}

func TestCheck_readsThatBringNothingFromOutside_areNotAPath(t *testing.T) {
	t.Parallel()

	// An agent that reads only what this installation authored and then writes
	// is doing the ordinary thing. Reporting it would make the check noise.
	got := flow.Check([]domain.ToolID{"kb.search", "crm.reply"}, nil, known)

	if len(got.Paths) != 0 {
		t.Errorf("paths = %+v, want none", got.Paths)
	}
}

func TestCheck_theWriteBeforeTheRead_isNotAPath(t *testing.T) {
	t.Parallel()

	// This is the whole reason envelopes exist. A specification that writes
	// first and reads afterwards cannot carry the read's taint into the write,
	// and a check that ignored the order would tell every author their agent
	// is dangerous.
	got := flow.Check(nil, []flow.Envelope{
		{Name: "Responder", Reaches: []domain.ToolID{"crm.reply"}},
		{Name: "Consultar", Reaches: []domain.ToolID{"crm.lookup"}},
	}, known)

	if len(got.Paths) != 0 {
		t.Errorf("paths = %+v, want none: the write happens first", got.Paths)
	}
}

func TestCheck_taintFromAnEarlierStep_reachesALaterWrite(t *testing.T) {
	t.Parallel()

	got := flow.Check(nil, []flow.Envelope{
		{Name: "Consultar", Reaches: []domain.ToolID{"crm.lookup"}},
		{Name: "Pensar"},
		{Name: "Responder", Reaches: []domain.ToolID{"crm.reply"}},
	}, known)

	if len(got.Paths) != 1 {
		t.Fatalf("paths = %+v, want one across the steps", got.Paths)
	}
	if got.Paths[0].FromStep != "Consultar" || got.Paths[0].ToStep != "Responder" {
		t.Errorf("path = %+v, want it to name both steps", got.Paths[0])
	}
}

func TestCheck_theWorstEffectItReaches_isReported(t *testing.T) {
	t.Parallel()

	// An author deciding whether to care needs to know it is money, not a
	// note. The effect is on the path for exactly that.
	got := flow.Check([]domain.ToolID{"crm.lookup", "pay.refund"}, nil, known)

	if len(got.Paths) != 1 || got.Paths[0].Effect != domain.EffectFinancial {
		t.Errorf("paths = %+v, want the financial effect named", got.Paths)
	}
}

func TestCheck_aToolNobodyClassified_isSaidRatherThanAssumed(t *testing.T) {
	t.Parallel()

	// It reads as read-only and untrusted, which is the safe default and also
	// means this check cannot say anything true about it. Saying nothing would
	// let an unclassified financial tool publish looking harmless.
	got := flow.Check([]domain.ToolID{"crm.lookup", "algo.novo"}, nil, known)

	if len(got.Unclassified) != 1 || got.Unclassified[0] != "algo.novo" {
		t.Errorf("unclassified = %v, want the unknown tool named", got.Unclassified)
	}
	if got.Clean() {
		t.Error("Clean() = true with a tool nobody ruled on")
	}
}
