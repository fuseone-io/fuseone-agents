package guard

import (
	"errors"
	"net/http"
	"testing"
)

func (h *harness) merger() *Merger {
	return NewMerger(h.cfg, h.client("merger-token"))
}

func (h *harness) mergeIfReady(headSHA string) MergeResult {
	h.t.Helper()
	res, err := h.merger().MergeIfReady(h.t.Context(), MergeInput{PR: testPR, HeadSHA: headSHA})
	if err != nil {
		h.t.Fatalf("merge_if_ready: %v", err)
	}
	return res
}

// refusedBeforeMerging asserts the reason and, as the anchor, that the merge
// endpoint was never called: the fake's ruleset would refuse many of these
// too, so the reason alone would not prove the guard stopped first.
func (h *harness) refusedBeforeMerging(res MergeResult, want Reason) {
	h.t.Helper()
	if res.Merged || res.Reason != want {
		h.t.Fatalf("result = %+v, want refused as %s", res, want)
	}
	if len(h.fake.attempts) != 0 {
		h.t.Fatalf("merge was attempted: %+v", h.fake.attempts)
	}
}

// setReviewers changes the one list both the guard and the ruleset read.
func (h *harness) setReviewers(logins ...string) {
	h.cfg.Reviewers = logins
	h.edit(func(f *fakeGitHub) { f.ruleset.Reviewers = logins })
}

func TestMergeIfReady_allValid_mergedWithExpectedHeadSHA(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	res := h.mergeIfReady(headA)

	if !res.Merged || res.Reason != ReasonMerged || res.MergeSHA != mergeSHA || res.HeadSHA != headA {
		t.Fatalf("result = %+v", res)
	}
	want := mergeCall{PR: testPR, SHA: headA, Method: "squash"}
	if len(h.fake.attempts) != 1 || h.fake.attempts[0] != want {
		t.Fatalf("merge calls = %+v, want exactly %+v", h.fake.attempts, want)
	}
}

func TestMergeIfReady_headMovedAfterApproval_refusedNothingMerged(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.edit(func(f *fakeGitHub) { f.prs[testPR].Head = headB })
	h.refusedBeforeMerging(h.mergeIfReady(headA), ReasonHeadMoved)
}

func TestMergeIfReady_review_mustBeAnAuthorisedApprovalOfThisHead(t *testing.T) {
	t.Parallel()

	reviewer := "fuseone-factory-reviewer[bot]"
	cases := []struct {
		name    string
		reviews []fakeReview
		want    Reason
	}{
		{"approval on an older sha", []fakeReview{
			{ID: 1, Login: reviewer, State: "APPROVED", CommitID: headB}}, ReasonReviewStale},
		{"approval by a non-authorised account", []fakeReview{
			{ID: 1, Login: "mallory", State: "APPROVED", CommitID: headA}}, ReasonReviewMissing},
		{"no review at all", nil, ReasonReviewMissing},
		{"a comment is not an approval", []fakeReview{
			{ID: 1, Login: reviewer, State: "COMMENTED", CommitID: headA}}, ReasonReviewMissing},
		{"approval dismissed", []fakeReview{
			{ID: 1, Login: reviewer, State: "APPROVED", CommitID: headA},
			{ID: 2, Login: reviewer, State: "DISMISSED", CommitID: headA}}, ReasonReviewMissing},
		{"changes requested by an authorised reviewer", []fakeReview{
			{ID: 1, Login: reviewer, State: "APPROVED", CommitID: headA},
			{ID: 2, Login: "klinux", State: "CHANGES_REQUESTED", CommitID: headA}}, ReasonChangesRequested},
		{"approval then changes requested by the same reviewer", []fakeReview{
			{ID: 1, Login: reviewer, State: "APPROVED", CommitID: headA},
			{ID: 2, Login: reviewer, State: "CHANGES_REQUESTED", CommitID: headA}}, ReasonChangesRequested},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.edit(func(f *fakeGitHub) { f.reviews[testPR] = tc.reviews })
			h.refusedBeforeMerging(h.mergeIfReady(headA), tc.want)
		})
	}
}

// A later comment does not undo an approval, on GitHub or here.
func TestMergeIfReady_commentAfterApproval_stillMerges(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.edit(func(f *fakeGitHub) {
		f.reviews[testPR] = append(f.reviews[testPR],
			fakeReview{ID: 2, Login: "fuseone-factory-reviewer[bot]", State: "COMMENTED", CommitID: headA})
	})
	if res := h.mergeIfReady(headA); !res.Merged {
		t.Fatalf("result = %+v", res)
	}
}

// The authorised set is the configured list and nothing else: no built-in
// login, no fallback. The fake's ruleset reads the same list (#207).
func TestMergeIfReady_reviewerSet_isExactlyTheConfiguredList(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		configured []string
		author     string
		merged     bool
	}{
		{"author configured", []string{"alice"}, "alice", true},
		{"author not configured", []string{"bob"}, "alice", false},
		{"logins compare case-insensitively, as GitHub's do", []string{"Alice"}, "alice", true},
		{"the default reviewer is not implied", []string{"bob"}, "fuseone-factory-reviewer[bot]", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.setReviewers(tc.configured...)
			h.edit(func(f *fakeGitHub) {
				f.reviews[testPR] = []fakeReview{{ID: 1, Login: tc.author, State: "APPROVED", CommitID: headA}}
			})
			res := h.mergeIfReady(headA)
			if res.Merged != tc.merged {
				t.Fatalf("result = %+v, want merged=%v", res, tc.merged)
			}
			if !tc.merged {
				h.refusedBeforeMerging(res, ReasonReviewMissing)
			}
		})
	}
}

func TestMergeIfReady_qa_mustBeASuccessStatusFromTheQAApp(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		statuses map[string][]fakeStatus
		labels   []string
		want     Reason
	}{
		{"recorded as a label only", nil, []string{"factory:qa-passed"}, ReasonQAMissing},
		{"status from a person", map[string][]fakeStatus{headA: {
			{ID: 10, State: "success", Context: QAContext, Creator: intruder}}}, nil, ReasonQAUntrustedCreator},
		{"status from another app", map[string][]fakeStatus{headA: {
			{ID: 10, State: "success", Context: QAContext, Creator: mergerBot}}}, nil, ReasonQAUntrustedCreator},
		{"QA's success shadowed by a later one from someone else", map[string][]fakeStatus{headA: {
			{ID: 10, State: "success", Context: QAContext, Creator: qaBot},
			{ID: 11, State: "success", Context: QAContext, Creator: intruder}}}, nil, ReasonQAUntrustedCreator},
		{"QA failed", map[string][]fakeStatus{headA: {
			{ID: 10, State: "failure", Context: QAContext, Creator: qaBot}}}, nil, ReasonQANotSuccess},
		{"QA passed earlier then failed", map[string][]fakeStatus{headA: {
			{ID: 10, State: "success", Context: QAContext, Creator: qaBot},
			{ID: 11, State: "failure", Context: QAContext, Creator: qaBot}}}, nil, ReasonQANotSuccess},
		{"QA passed only an older head", map[string][]fakeStatus{headB: {
			{ID: 10, State: "success", Context: QAContext, Creator: qaBot}}}, nil, ReasonQAMissing},
		{"another context from QA is not the verdict", map[string][]fakeStatus{headA: {
			{ID: 10, State: "success", Context: "factory/qa-draft", Creator: qaBot}}}, nil, ReasonQAMissing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.edit(func(f *fakeGitHub) {
				f.statuses = map[string][]fakeStatus{}
				for sha, list := range tc.statuses {
					f.statuses[sha] = list
				}
				f.prs[testPR].Labels = append(f.prs[testPR].Labels, tc.labels...)
			})
			h.refusedBeforeMerging(h.mergeIfReady(headA), tc.want)
		})
	}
}

func TestMergeIfReady_checks_everyExpectedOneGreenFromTheExpectedApp(t *testing.T) {
	t.Parallel()

	goOK := fakeCheck{ID: 20, Name: "go", Status: "completed", Conclusion: "success", AppID: actionsAppID, HeadSHA: headA}
	pg := func(status, conclusion string, app int64) fakeCheck {
		return fakeCheck{ID: 21, Name: "postgres", Status: status, Conclusion: conclusion, AppID: app, HeadSHA: headA}
	}
	cases := []struct {
		name   string
		checks []fakeCheck
		want   Reason
	}{
		{"one missing", []fakeCheck{goOK}, ReasonCheckMissing},
		{"emitted by an unauthorised app", []fakeCheck{goOK, pg("completed", "success", 999)}, ReasonCheckMissing},
		{"still running", []fakeCheck{goOK, pg("in_progress", "", actionsAppID)}, ReasonCheckNotSuccess},
		{"failed", []fakeCheck{goOK, pg("completed", "failure", actionsAppID)}, ReasonCheckNotSuccess},
		{"skipped is not success", []fakeCheck{goOK, pg("completed", "skipped", actionsAppID)}, ReasonCheckNotSuccess},
		{"an impostor's success does not cover the real failure", []fakeCheck{
			goOK, pg("completed", "failure", actionsAppID),
			{ID: 30, Name: "postgres", Status: "completed", Conclusion: "success", AppID: 999, HeadSHA: headA},
		}, ReasonCheckNotSuccess},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.edit(func(f *fakeGitHub) { f.checks[headA] = tc.checks })
			res := h.mergeIfReady(headA)
			h.refusedBeforeMerging(res, tc.want)
			if res.Check != "postgres" {
				t.Fatalf("failing check = %q, want postgres", res.Check)
			}
		})
	}
}

// expectedHeadSha closes a push; GitHub's ruleset closes a signal that changes
// on the same commit between the guard's read and the merge (#207). The guard
// must report that refusal, fail-closed, and not retry.
func TestMergeIfReady_signalChangesOnTheSameSHAAfterTheRead_mergeRefusedAndReported(t *testing.T) {
	t.Parallel()

	cases := map[string]func(f *fakeGitHub){
		"review dismissed": func(f *fakeGitHub) { f.reviews[testPR][0].State = "DISMISSED" },
		"check back to pending": func(f *fakeGitHub) {
			f.checks[headA][1].Status, f.checks[headA][1].Conclusion = "queued", ""
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.edit(func(f *fakeGitHub) { f.beforeMerge = change })
			res := h.mergeIfReady(headA)
			if res.Merged || res.Reason != ReasonMergeRefused || res.HTTPStatus != http.StatusMethodNotAllowed {
				t.Fatalf("result = %+v, want merge_refused with 405", res)
			}
			if len(h.fake.attempts) != 1 || h.fake.attempts[0].SHA != headA || h.fake.prs[testPR].Merged {
				t.Fatalf("attempts %+v, merged %v: want one refused attempt on headA, no retry",
					h.fake.attempts, h.fake.prs[testPR].Merged)
			}
		})
	}
}

func TestMergeIfReady_pushBetweenReadAndMerge_refusedByExpectedHeadSHA(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.edit(func(f *fakeGitHub) { f.beforeMerge = func(f *fakeGitHub) { f.prs[testPR].Head = headB } })
	res := h.mergeIfReady(headA)
	if res.Merged || res.Reason != ReasonHeadMoved || res.HTTPStatus != http.StatusConflict {
		t.Fatalf("result = %+v, want head_moved reported from a 409", res)
	}
	if len(h.fake.attempts) != 1 || h.fake.attempts[0].SHA != headA {
		t.Fatalf("attempts = %+v, want one, with sha=headA", h.fake.attempts)
	}
}

func TestMergeIfReady_githubRefusesTheMerge_reportedWithItsStatus(t *testing.T) {
	t.Parallel()

	for _, code := range []int{http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusMethodNotAllowed} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.edit(func(f *fakeGitHub) { f.mergeError = code })
			res := h.mergeIfReady(headA)
			if res.Merged || res.Reason != ReasonMergeRefused || res.HTTPStatus != code || len(h.fake.attempts) != 1 {
				t.Fatalf("result %+v after %d attempts, want merge_refused/%d once", res, len(h.fake.attempts), code)
			}
		})
	}
}

func TestMergeIfReady_prOutsideTheProtectedTarget_refused(t *testing.T) {
	t.Parallel()

	no := false
	cases := []struct {
		name   string
		mutate func(*fakePR)
		want   Reason
	}{
		{"targets another branch", func(p *fakePR) { p.BaseRef = "develop" }, ReasonWrongBase},
		{"in another repository", func(p *fakePR) { p.BaseRepo = "someone/fuseone-agents" }, ReasonWrongRepository},
		{"closed", func(p *fakePR) { p.State = "closed" }, ReasonPRNotOpen},
		{"conflicting", func(p *fakePR) { p.Mergeable = &no }, ReasonNotMergeable},
		{"mergeability not yet computed", func(p *fakePR) { p.Mergeable = nil }, ReasonMergeabilityUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.edit(func(f *fakeGitHub) { tc.mutate(f.prs[testPR]) })
			h.refusedBeforeMerging(h.mergeIfReady(headA), tc.want)
		})
	}
}

// A renamed or transferred repository answers with a redirect. Following it
// would act on a repository the configuration never named.
func TestMergeIfReady_repositoryRedirects_failsWithoutFollowing(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.edit(func(f *fakeGitHub) { f.renamed = true })
	_, err := h.merger().MergeIfReady(t.Context(), MergeInput{PR: testPR, HeadSHA: headA})
	if !errors.Is(err, ErrGitHub) || len(h.fake.attempts) != 0 {
		t.Fatalf("err %v, attempts %d: want a GitHub error and no merge", err, len(h.fake.attempts))
	}
}

// Strict status checks (#207): a head tested against an older main must be
// brought up to date and tested again before it merges.
func TestMergeIfReady_mainAdvancedAfterTheChecks_blockedUntilUpdatedAndRetested(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.edit(func(f *fakeGitHub) { f.behind[headA] = 3 })
	h.refusedBeforeMerging(h.mergeIfReady(headA), ReasonBaseBehind)

	// Updated: a new head, up to date, but nothing re-reviewed or re-tested.
	h.edit(func(f *fakeGitHub) { f.prs[testPR].Head = headB })
	h.refusedBeforeMerging(h.mergeIfReady(headB), ReasonReviewStale)
}

func TestMergeIfReady_invalidInput_refusedWithoutCallingGitHub(t *testing.T) {
	t.Parallel()

	for name, in := range map[string]MergeInput{
		"no pr":        {PR: 0, HeadSHA: headA},
		"short sha":    {PR: testPR, HeadSHA: "aaaa"},
		"a branch":     {PR: testPR, HeadSHA: "main"},
		"padded sha":   {PR: testPR, HeadSHA: " " + headA},
		"negative pr":  {PR: -1, HeadSHA: headA},
		"empty sha":    {PR: testPR},
		"sha with ref": {PR: testPR, HeadSHA: headA + "^"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			res, err := h.merger().MergeIfReady(t.Context(), in)
			if err != nil || res.Merged || res.Reason != ReasonInvalidInput || h.fake.requests != 0 {
				t.Fatalf("result %+v, err %v, %d requests: want invalid_input before any", res, err, h.fake.requests)
			}
		})
	}
}
