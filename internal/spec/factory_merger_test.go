package spec_test

import (
	"strings"
	"testing"
)

/*
The Merger proposes merge_if_ready, a destructive effect, and the Gate blocks
an irreversible effect outright in a run that read untrusted data; no approval
releases it. So the Merger reads nothing anyone wrote: its only read is the
factory server's list_ready_prs, which answers numbers, SHAs, item ids and
reasons (#199). These tests keep it that way: a hosted GitHub tool in its pack
would make every merge impossible, and a reworded fence would let a run reach
for one.
*/

// mergerPreClaimRule fences what the Merger does before its claim.
var mergerPreClaimRule = []string{
	"Before the claim, call only list_ready_prs; it is the only read you have, and it returns numbers, commit SHAs, item ids and reasons, nothing anyone wrote.",
	"Take the oldest pull request it lists with ready true, or, if none is ready, the oldest whose reason send_back takes, and claim it before anything else.",
}

// mergerSealedLines are its fences and its one way into Travado, each
// required exactly once.
var mergerSealedLines = []string{
	"Never read a pull request, an issue, a comment or a file: you act only on numbers, commit SHAs, item ids and reasons.",
	"Never merge any other way than merge_if_ready, and never propose it for a pull request list_ready_prs did not list as ready.",
	"Block a card only through send_back, which writes the comment that names the action a person must take and ends it with Return to: Doing, and then move the card to Travado with move_card.",
}

func TestFactoryReferences_merger_carriesThePreClaimRuleBeforeItsClaim(t *testing.T) {
	t.Parallel()

	assertLinesBeforeClaim(t, "merger", mergerPreClaimRule)
}

func TestFactoryReferences_merger_carriesItsFencesOnce(t *testing.T) {
	t.Parallel()

	data, ok := factoryReferences(t)["merger"]
	if !ok {
		t.Fatal("there is no Merger reference")
	}
	lines := strings.Split(string(data), "\n")
	for _, sentence := range mergerSealedLines {
		if count := countLines(lines, sentence); count != 1 {
			t.Errorf("the Merger reference carries the line %q %d times, want 1", sentence, count)
		}
	}
}

// Every tool the Merger declares is the claim or its own factory server's:
// no hosted GitHub tool, whose answer would taint the run before the merge.
func TestFactoryReferences_merger_declaresNoToolThatReadsText(t *testing.T) {
	t.Parallel()

	s, ok := parsedReferences(t)["merger"]
	if !ok {
		t.Fatal("there is no Merger reference")
	}
	if len(s.Tools) < 5 {
		t.Fatalf("the Merger declares %v; the scan is broken", s.Tools)
	}
	for _, tool := range s.Tools {
		if tool != "$fuseone.work_item.claim" && !strings.HasPrefix(string(tool), "factory-merger.") {
			t.Errorf("the Merger declares %s; its only tools are the claim and factory-merger's", tool)
		}
	}
}
