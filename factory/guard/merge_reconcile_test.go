package guard

import "testing"

// A merge GitHub carried out but whose response was lost is still a merge.
// Reported as an error, the destructive effect would have happened while the
// ledger and the card never learnt it, and the next attempt would only find
// the PR closed. The guard re-reads the PR and settles the outcome.
func TestMergeIfReady_mergedButResponseLost_reconciledAsMerged(t *testing.T) {
	t.Parallel()

	for _, fault := range []string{faultTruncate, faultDrop} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.edit(func(f *fakeGitHub) { f.mergeFault = fault })

			res, err := h.merger().MergeIfReady(t.Context(), MergeInput{PR: testPR, HeadSHA: headA})
			if err != nil {
				t.Fatalf("merge_if_ready returned an error for a merge that happened: %v", err)
			}
			if !res.Merged || res.Reason != ReasonMerged || res.MergeSHA != mergeSHA {
				t.Fatalf("result = %+v, want merged with the merge commit", res)
			}
			if len(h.fake.attempts) != 1 {
				t.Fatalf("merge calls = %d, want exactly one: reconciling reads, it never merges again", len(h.fake.attempts))
			}
		})
	}
}

// When the re-read fails too, the outcome is said to be unknown, structured,
// never a bare error and never a guess either way.
func TestMergeIfReady_responseLostAndPRUnreadable_outcomeUnknown(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.edit(func(f *fakeGitHub) { f.mergeFault, f.readsDown = faultDrop, true })

	res, err := h.merger().MergeIfReady(t.Context(), MergeInput{PR: testPR, HeadSHA: headA})
	if err != nil {
		t.Fatalf("merge_if_ready returned a bare error: %v", err)
	}
	if res.Merged || res.Reason != ReasonOutcomeUnknown || res.HeadSHA != headA {
		t.Fatalf("result = %+v, want outcome_unknown", res)
	}
}

// The approval resumes, the first execution merged but its result never
// reached the run, and the tool is called again: it must say merged.
func TestMergeIfReady_retryAfterASuccessfulMerge_reportsMerged(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	if first := h.mergeIfReady(headA); !first.Merged {
		t.Fatalf("first call = %+v", first)
	}
	again := h.mergeIfReady(headA)
	if !again.Merged || again.Reason != ReasonMerged || again.MergeSHA != mergeSHA {
		t.Fatalf("retry = %+v, want merged, not pr_not_open", again)
	}
	if len(h.fake.attempts) != 1 {
		t.Fatalf("merge calls = %d, want one", len(h.fake.attempts))
	}
}

// Merged is claimed only for the head that was proposed. A PR merged after a
// push carried code nobody approved through this call.
func TestMergeIfReady_prMergedAtAnotherHead_notReportedAsMerged(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.edit(func(f *fakeGitHub) {
		pr := f.prs[testPR]
		pr.Head, pr.State, pr.Merged, pr.MergeCommit = headB, "closed", true, mergeSHA
	})
	h.refusedBeforeMerging(h.mergeIfReady(headA), ReasonPRNotOpen)
}

// Reconciling claims merged only for this call's head: if the response was
// lost and the re-read shows the PR still open, or merged with another head,
// the outcome is unknown, not merged.
func TestMergeIfReady_responseLostAndNotMergedAtThisHead_outcomeUnknown(t *testing.T) {
	t.Parallel()

	cases := map[string]func(f *fakeGitHub){
		"still open": func(*fakeGitHub) {},
		"merged at another head": func(f *fakeGitHub) {
			pr := f.prs[testPR]
			pr.Head, pr.State, pr.Merged, pr.MergeCommit = headB, "closed", true, mergeSHA
		},
	}
	for name, meanwhile := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.edit(func(f *fakeGitHub) { f.mergeFault, f.beforeMerge = faultDropUnmerged, meanwhile })
			res, err := h.merger().MergeIfReady(t.Context(), MergeInput{PR: testPR, HeadSHA: headA})
			if err != nil {
				t.Fatalf("merge_if_ready returned a bare error: %v", err)
			}
			if res.Merged || res.Reason != ReasonOutcomeUnknown {
				t.Fatalf("result = %+v, want outcome_unknown", res)
			}
		})
	}
}
