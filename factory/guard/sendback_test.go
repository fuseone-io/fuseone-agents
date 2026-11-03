package guard

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// mergerTools builds merge_if_ready and send_back over one refusal record,
// on a clock the test moves.
func (h *harness) mergerTools(clock Clock) (*Merger, *SendBack) {
	return NewMergerTools(h.cfg, h.client("merger-token"), clock)
}

func (h *harness) sendBackWith(s *SendBack, cause Reason) SendBackResult {
	h.t.Helper()
	res, err := s.SendBack(h.t.Context(), SendBackInput{PR: testPR, HeadSHA: headA, Reason: string(cause)})
	if err != nil {
		h.t.Fatalf("send_back: %v", err)
	}
	return res
}

// sendBack makes cause true on GitHub (or, for a merge's own refusal, has
// merge_if_ready observe it), then sends it back.
func (h *harness) sendBack(cause Reason) SendBackResult {
	h.t.Helper()
	merger, s := h.mergerTools(nil)
	h.edit(causes[cause])
	if unrebuildable(cause) {
		if res, err := merger.MergeIfReady(h.t.Context(), MergeInput{PR: testPR, HeadSHA: headA}); err != nil || res.Reason != cause {
			h.t.Fatalf("merge_if_ready = %+v, %v; want %s observed", res, err, cause)
		}
		h.edit(func(f *fakeGitHub) { f.mergeError, f.mergeFault = 0, "" })
	}
	return h.sendBackWith(s, cause)
}

// causes makes each cause true on the harness's otherwise ready PR.
var causes = map[Reason]func(f *fakeGitHub){
	ReasonHeadMoved:          func(f *fakeGitHub) { f.prs[testPR].Head = headB },
	ReasonReviewMissing:      func(f *fakeGitHub) { f.reviews[testPR] = nil },
	ReasonReviewStale:        func(f *fakeGitHub) { f.reviews[testPR][0].CommitID = headB },
	ReasonChangesRequested:   func(f *fakeGitHub) { f.reviews[testPR][0].State = "CHANGES_REQUESTED" },
	ReasonQANotSuccess:       func(f *fakeGitHub) { f.statuses[headA][0].State = "failure" },
	ReasonCheckNotSuccess:    func(f *fakeGitHub) { f.checks[headA][0].Conclusion = "failure" },
	ReasonQAMissing:          func(f *fakeGitHub) { f.statuses[headA] = nil },
	ReasonQAUntrustedCreator: func(f *fakeGitHub) { f.statuses[headA][0].Creator = intruder },
	ReasonBaseBehind:         func(f *fakeGitHub) { f.behind[headA] = 1 },
	ReasonNotMergeable:       func(f *fakeGitHub) { no := false; f.prs[testPR].Mergeable = &no },
	ReasonMergeRefused:       func(f *fakeGitHub) { f.mergeError = 405 },
	ReasonOutcomeUnknown:     func(f *fakeGitHub) { f.mergeFault = faultDropUnmerged },
}

func (h *harness) prLabels() []string {
	h.fake.mu.Lock()
	defer h.fake.mu.Unlock()
	return slices.Clone(h.fake.prs[testPR].Labels)
}

// sendBackTable is #199's mapping written out by hand: where each refusal
// sends the pull request, and whether its card goes to Travado instead.
var sendBackTable = map[Reason]struct {
	to      string // the sub-state label that replaces ready-to-merge; "" keeps it
	travado bool
}{
	ReasonHeadMoved:          {LabelNeedsReview, false},
	ReasonReviewMissing:      {LabelNeedsReview, false},
	ReasonReviewStale:        {LabelNeedsReview, false},
	ReasonChangesRequested:   {LabelChangesRequested, false},
	ReasonQANotSuccess:       {LabelChangesRequested, false},
	ReasonCheckNotSuccess:    {LabelChangesRequested, false},
	ReasonQAMissing:          {LabelNeedsQA, false},
	ReasonQAUntrustedCreator: {LabelNeedsQA, false},
	ReasonBaseBehind:         {"", true},
	ReasonNotMergeable:       {"", true},
	ReasonMergeRefused:       {"", true},
	ReasonOutcomeUnknown:     {"", true},
}

func TestSendBack_eachReason_movesTheLabelOrBlocksTheCard(t *testing.T) {
	t.Parallel()

	for cause, want := range sendBackTable {
		t.Run(string(cause), func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			res := h.sendBack(cause)
			if !res.SentBack || res.Reason != ReasonSentBack || res.Travado != want.travado {
				t.Fatalf("result %+v, want sent back with travado %v", res, want.travado)
			}
			labels := h.prLabels()
			if want.to == "" {
				if !slices.Equal(labels, []string{LabelReadyToMerge}) || len(h.fake.labelOps) != 0 {
					t.Fatalf("labels %v after %v, want ready-to-merge kept untouched", labels, h.fake.labelOps)
				}
				return
			}
			if !slices.Equal(labels, []string{want.to}) {
				t.Fatalf("labels %v, want exactly %q", labels, want.to)
			}
		})
	}
}

// Every comment is built from closed text: it names what failed and what a
// role or a person does next, and a blocking one ends with its return line.
func TestSendBack_comment_isClosedTextEndingInItsReturnLine(t *testing.T) {
	t.Parallel()

	for cause, want := range sendBackTable {
		t.Run(string(cause), func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			res := h.sendBack(cause)
			if len(h.fake.comments) != 1 {
				t.Fatalf("%d comments posted, want one", len(h.fake.comments))
			}
			c := h.fake.comments[0]
			if c.ID != res.CommentID || c.Author != mergerBot || c.Issue != testPR {
				t.Fatalf("comment %+v, result %+v", c, res)
			}
			if !strings.Contains(c.Body, string(cause)) || !strings.Contains(c.Body, headA[:12]) {
				t.Fatalf("comment does not name the reason and the head: %q", c.Body)
			}
			if ends := strings.HasSuffix(strings.TrimSpace(c.Body), "Return to: Doing"); ends != want.travado {
				t.Fatalf("comment ends with the return line = %v, want %v: %q", ends, want.travado, c.Body)
			}
		})
	}
}

// The new label goes on before ready-to-merge comes off: a failure between
// the two leaves both, which the next send_back repairs, never a pull request
// with no sub-state at all.
func TestSendBack_labels_addBeforeRemove(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.sendBack(ReasonReviewStale)
	want := []string{"+" + LabelNeedsReview, "-" + LabelReadyToMerge}
	if !slices.Equal(h.fake.labelOps, want) {
		t.Fatalf("label operations %v, want %v", h.fake.labelOps, want)
	}
}

func TestSendBack_refusals_writeNothing(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		cause  Reason
		mutate func(f *fakeGitHub)
		want   Reason
	}{
		"not ready to merge": {ReasonReviewStale, func(f *fakeGitHub) { f.prs[testPR].Labels = nil }, ReasonNotReadyToMerge},
		"closed":             {ReasonReviewStale, func(f *fakeGitHub) { f.prs[testPR].State = "closed" }, ReasonPRNotOpen},
		"wrong base":         {ReasonReviewStale, func(f *fakeGitHub) { f.prs[testPR].BaseRef = "develop" }, ReasonWrongBase},
		"transient reason":   {ReasonCheckMissing, func(*fakeGitHub) {}, ReasonInvalidInput},
		"unknown reason":     {"no_reason", func(*fakeGitHub) {}, ReasonInvalidInput},
		"merged is no cause": {ReasonMerged, func(*fakeGitHub) {}, ReasonInvalidInput},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			_, s := h.mergerTools(nil)
			h.edit(causes[ReasonReviewStale])
			h.edit(c.mutate)
			res := h.sendBackWith(s, c.cause)
			if res.SentBack || res.Reason != c.want {
				t.Fatalf("result %+v, want refused with %q", res, c.want)
			}
			if len(h.fake.comments) != 0 || len(h.fake.labelOps) != 0 {
				t.Fatalf("a refusal wrote: comments %v, labels %v", h.fake.comments, h.fake.labelOps)
			}
		})
	}
}

func TestSendBack_commentRefused_changesNoLabel(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.edit(func(f *fakeGitHub) { f.writeError = 403 })
	res := h.sendBack(ReasonReviewStale)
	if res.SentBack || res.Reason != ReasonCommentRefused || res.HTTPStatus != 403 {
		t.Fatalf("result %+v, want comment_refused with 403", res)
	}
	if len(h.fake.labelOps) != 0 {
		t.Fatalf("labels changed after the comment was refused: %v", h.fake.labelOps)
	}
}

type movableClock struct{ now time.Time }

func (c *movableClock) Now() time.Time { return c.now }

// The caller's reason is never proof: on a PR merge_if_ready would merge,
// every cause is refused and nothing is written.
func TestSendBack_readyPullRequest_everyCauseRefusedAndNothingWritten(t *testing.T) {
	t.Parallel()

	for cause := range sendBackTable {
		t.Run(string(cause), func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			_, s := h.mergerTools(nil)
			res := h.sendBackWith(s, cause)
			if res.SentBack || res.Reason != ReasonCauseNotObserved || res.Travado {
				t.Fatalf("result %+v, want cause_not_observed", res)
			}
			if len(h.fake.comments) != 0 || len(h.fake.labelOps) != 0 {
				t.Fatalf("an unobserved cause wrote: comments %v, labels %v", h.fake.comments, h.fake.labelOps)
			}
		})
	}
}

// The listing said review_stale; before send_back the Reviewer approved the
// head. The stale diagnosis is not written.
func TestSendBack_signalRepairedAfterTheListing_refused(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	_, s := h.mergerTools(nil)
	h.edit(causes[ReasonReviewStale])
	h.edit(func(f *fakeGitHub) { f.reviews[testPR][0].CommitID = headA })
	if res := h.sendBackWith(s, ReasonReviewStale); res.Reason != ReasonCauseNotObserved {
		t.Fatalf("result %+v, want cause_not_observed", res)
	}
}

// A different cause than the one GitHub shows now is refused: QA missing is
// not sent back as changes requested.
func TestSendBack_anotherCauseThanTheObservedOne_refused(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	_, s := h.mergerTools(nil)
	h.edit(causes[ReasonQAMissing])
	if res := h.sendBackWith(s, ReasonChangesRequested); res.Reason != ReasonCauseNotObserved {
		t.Fatalf("result %+v, want cause_not_observed", res)
	}
}

// merge_refused and outcome_unknown are accepted only as this server's own
// observation of that head, within the hour.
func TestSendBack_unrebuildableCause_needsTheServersOwnObservation(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	cases := map[string]func(h *harness, clock *movableClock, other *SendBack) *SendBack{
		"another instance observed it": func(h *harness, _ *movableClock, other *SendBack) *SendBack {
			_, fresh := h.mergerTools(nil)
			return fresh
		},
		"observed more than an hour ago": func(_ *harness, clock *movableClock, other *SendBack) *SendBack {
			clock.now = start.Add(refusalTTL)
			return other
		},
		"the head moved since": func(h *harness, _ *movableClock, other *SendBack) *SendBack {
			h.edit(func(f *fakeGitHub) { f.prs[testPR].Head = headB })
			return other
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			clock := &movableClock{now: start}
			merger, s := h.mergerTools(clock)
			h.edit(causes[ReasonMergeRefused])
			if res, _ := merger.MergeIfReady(t.Context(), MergeInput{PR: testPR, HeadSHA: headA}); res.Reason != ReasonMergeRefused {
				t.Fatalf("merge_if_ready = %+v", res)
			}
			if res := h.sendBackWith(change(h, clock, s), ReasonMergeRefused); res.Reason != ReasonCauseNotObserved {
				t.Fatalf("result %+v, want cause_not_observed", res)
			}
			if len(h.fake.comments) != 0 {
				t.Fatalf("comments %v", h.fake.comments)
			}
		})
	}
}
