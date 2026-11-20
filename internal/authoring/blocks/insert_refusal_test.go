package blocks_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/authoring/blocks"
)

/*
An insert refused says which part was wrong, in words the card can show (#413). The model
named kinds the codec does not have ("default", "always") and positions past the end of the
document, and the card only said "no such block, kind or position: insert default at 1".
*/
func TestInsert_refusal_namesTheKindsThereAreOrThePositionsThereAre(t *testing.T) {
	t.Parallel()
	doc := blocks.Parse("Objetivo\nTriar.")
	for name, tc := range map[string]struct {
		at   int
		kind blocks.Kind
		want []string
	}{
		"an unknown kind":  {0, "default", []string{`"default"`, "prose", "objective", "howToAct", "whenToStop", "never", "howToReply"}},
		"past the end":     {2, blocks.Never, []string{"position 2", "0 to 1"}},
		"before the start": {-1, blocks.Never, []string{"position -1", "0 to 1"}},
	} {
		_, err := blocks.Insert(doc, tc.at, tc.kind, "x", "pt-BR")
		if !errors.Is(err, blocks.ErrUnknownBlock) {
			t.Fatalf("%s: Insert = %v, want ErrUnknownBlock", name, err)
		}
		for _, want := range tc.want {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: %q does not say %s", name, err, want)
			}
		}
	}
}

// Every kind there is, in the order the prompt lists them, prose first.
func TestKinds_areEveryKindTheCodecWrites(t *testing.T) {
	t.Parallel()
	got := blocks.Kinds()
	want := []blocks.Kind{blocks.Prose, blocks.Objective, blocks.HowToAct, blocks.WhenToStop, blocks.Never, blocks.HowToReply}
	if len(got) != len(want) {
		t.Fatalf("Kinds = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] || !blocks.KindOf(got[i]) {
			t.Errorf("Kinds[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}
