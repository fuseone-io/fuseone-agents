package guard

import (
	"slices"
	"strings"
	"testing"
)

func (h *harness) sendBack(cause Reason) SendBackResult {
	h.t.Helper()
	res, err := NewSendBack(h.cfg, h.client("merger-token")).SendBack(h.t.Context(),
		SendBackInput{PR: testPR, HeadSHA: headA, Reason: string(cause)})
	if err != nil {
		h.t.Fatalf("send_back: %v", err)
	}
	return res
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
			h.edit(c.mutate)
			res := h.sendBack(c.cause)
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
