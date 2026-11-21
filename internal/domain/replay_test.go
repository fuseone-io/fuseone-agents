package domain_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func entry(position int, tool domain.ToolID, args, answer string) domain.WorldEntry {
	return domain.WorldEntry{Position: position, Tool: tool, ArgsDigest: args, AnswerRef: answer}
}

/*
A replayed call is matched against the next entry the world has not handed out yet, strictly
(NT-018 phase 5, D4): the next entry is consumed; one already handed out is a repetition; one
further on is a call out of order; nothing at all is a call the world never answered. When the
same call stands both before and after the next entry, it is a repetition: the run asked again
for what it was given, which is the divergence that already happened.
*/
func TestCaseWorld_Match_followsTheOrderOfPrecedence(t *testing.T) {
	t.Parallel()
	world := domain.CaseWorld{Case: "reg-1", Entries: []domain.WorldEntry{
		entry(1, "crm.lookup", "a", "r1"),
		entry(2, "crm.reply", "b", "r2"),
		entry(3, "crm.lookup", "a", "r3"),
		entry(4, "kb.search", "c", "r4"),
	}}
	for _, tc := range []struct {
		name     string
		consumed int
		tool     domain.ToolID
		args     string
		want     domain.Divergence
		answer   string
	}{
		{"the next entry is consumed", 0, "crm.lookup", "a", domain.DivergenceNone, "r1"},
		{"an entry already handed out is repeated", 2, "crm.reply", "b", domain.DivergenceRepeated, ""},
		{"an entry further on is out of order", 1, "kb.search", "c", domain.DivergenceOutOfOrder, ""},
		{"a call matching nothing is extra", 1, "crm.refund", "z", domain.DivergenceExtra, ""},
		{"the same tool with other arguments is extra", 0, "crm.lookup", "other", domain.DivergenceExtra, ""},
		{"a new call once the world is exhausted is extra", 4, "crm.note", "d", domain.DivergenceExtra, ""},
		{"an old call once the world is exhausted is repeated", 4, "crm.lookup", "a", domain.DivergenceRepeated, ""},
		{"before and after the next entry, repeated wins", 1, "crm.lookup", "a", domain.DivergenceRepeated, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, divergence := world.Match(tc.consumed, tc.tool, tc.args)
			if divergence != tc.want || got.AnswerRef != tc.answer {
				t.Errorf("Match = %q %q, want %q %q", got.AnswerRef, divergence, tc.answer, tc.want)
			}
		})
	}
}

// The same call made twice with different answers is two entries: each is handed out once,
// and in the order they were recorded.
func TestCaseWorld_Match_theSameCallTwice_handsOutEachAnswerOnceInOrder(t *testing.T) {
	t.Parallel()
	world := domain.CaseWorld{Case: "reg-1", Entries: []domain.WorldEntry{
		entry(1, "crm.lookup", "a", "first"),
		entry(2, "crm.lookup", "a", "second"),
	}}
	for consumed, want := range []string{"first", "second"} {
		got, divergence := world.Match(consumed, "crm.lookup", "a")
		if divergence != domain.DivergenceNone || got.AnswerRef != want {
			t.Errorf("call %d: Match = %q %q, want %q consumed", consumed+1, got.AnswerRef, divergence, want)
		}
	}
	if _, divergence := world.Match(2, "crm.lookup", "a"); divergence != domain.DivergenceRepeated {
		t.Errorf("a third call: %q, want it repeated", divergence)
	}
}

func TestStepReplayDiverged_isAKnownKind(t *testing.T) {
	t.Parallel()
	if !domain.StepReplayDiverged.Valid() {
		t.Error("replay_diverged is not a known step kind")
	}
}
