package guard

import (
	"strings"
	"testing"
)

func validVerdict(verdict string) VerdictInput {
	return VerdictInput{PR: testPR, HeadSHA: headA, Verdict: verdict, Summary: "all six criteria met", CommentID: 77}
}

func TestRecordQAVerdict_headMoved_refusedAndNoStatusSet(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.edit(func(f *fakeGitHub) { f.prs[testPR].Head = headB })

	res, err := h.qa("qa-token").Record(t.Context(), validVerdict("success"))
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if res.Recorded || res.Reason != ReasonHeadMoved {
		t.Fatalf("result = %+v, want refused as head_moved", res)
	}
	if len(h.fake.posted) != 0 {
		t.Fatalf("a status was set on a head that moved: %+v", h.fake.posted)
	}
}

func TestRecordQAVerdict_verdict_setsThatStateOnExactlyTheHead(t *testing.T) {
	t.Parallel()

	for _, verdict := range []string{"success", "failure"} {
		t.Run(verdict, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.edit(func(f *fakeGitHub) { f.statuses[headA] = nil })

			res, err := h.qa("qa-token").Record(t.Context(), validVerdict(verdict))
			if err != nil {
				t.Fatalf("record: %v", err)
			}
			if !res.Recorded || res.Reason != ReasonRecorded || res.State != verdict || res.HeadSHA != headA {
				t.Fatalf("result = %+v", res)
			}
			got := h.fake.statuses[headA]
			if len(got) != 1 || len(h.fake.posted) != 1 {
				t.Fatalf("statuses on head %+v, posted %+v: want exactly one, on headA", got, h.fake.posted)
			}
			s := got[0]
			wantURL := "https://github.com/fuseone-io/fuseone-agents/pull/42#issuecomment-77"
			if s.State != verdict || s.Context != "factory/qa" || s.Creator != qaBot ||
				s.Description != "all six criteria met" || s.TargetURL != wantURL {
				t.Fatalf("status = %+v", s)
			}
		})
	}
}

func TestRecordQAVerdict_invalidInput_refusedWithoutCallingGitHub(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*VerdictInput){
		"verdict not success or failure": func(in *VerdictInput) { in.Verdict = "approve" },
		"pending is not a verdict":       func(in *VerdictInput) { in.Verdict = "pending" },
		"short sha":                      func(in *VerdictInput) { in.HeadSHA = "aaaaaaa" },
		"uppercase sha":                  func(in *VerdictInput) { in.HeadSHA = strings.ToUpper(headA) },
		"empty summary":                  func(in *VerdictInput) { in.Summary = "  " },
		"no pr":                          func(in *VerdictInput) { in.PR = 0 },
		"negative comment":               func(in *VerdictInput) { in.CommentID = -1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			in := validVerdict("success")
			mutate(&in)
			res, err := h.qa("qa-token").Record(t.Context(), in)
			if err != nil {
				t.Fatalf("record: %v", err)
			}
			if res.Recorded || res.Reason != ReasonInvalidInput || h.fake.requests != 0 {
				t.Fatalf("result %+v after %d requests, want invalid_input before any", res, h.fake.requests)
			}
		})
	}
}

// GitHub caps a status description at 140 characters and refuses the whole
// status beyond it; a long summary must not cost QA its verdict.
func TestRecordQAVerdict_longSummary_truncatedToGitHubLimit(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	in := validVerdict("failure")
	in.Summary = strings.Repeat("é", 300) + "\nsecond line"

	res, err := h.qa("qa-token").Record(t.Context(), in)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if !res.Recorded {
		t.Fatalf("result = %+v", res)
	}
	desc := h.fake.posted[0].Description
	if n := len([]rune(desc)); n != 140 || strings.ContainsRune(desc, '\n') {
		t.Fatalf("description has %d runes (newline: %v), want 140 on one line", n, strings.ContainsRune(desc, '\n'))
	}
}

func TestRecordQAVerdict_prOutsideTheProtectedTarget_refused(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*fakePR)
		want   Reason
	}{
		{"another base", func(p *fakePR) { p.BaseRef = "release" }, ReasonWrongBase},
		{"another repository", func(p *fakePR) { p.BaseRepo = "someone/fork" }, ReasonWrongRepository},
		{"closed", func(p *fakePR) { p.State = "closed" }, ReasonPRNotOpen},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.edit(func(f *fakeGitHub) { tc.mutate(f.prs[testPR]) })
			res, err := h.qa("qa-token").Record(t.Context(), validVerdict("success"))
			if err != nil {
				t.Fatalf("record: %v", err)
			}
			if res.Recorded || res.Reason != tc.want || len(h.fake.posted) != 0 {
				t.Fatalf("result %+v, posted %d, want %s and nothing set", res, len(h.fake.posted), tc.want)
			}
		})
	}
}

// An instance wired with the wrong credential produces a factory/qa that the
// merge wrapper and the ruleset will both ignore. It must say so rather than
// report a verdict that counts for nothing.
func TestRecordQAVerdict_credentialIsNotTheQAApp_reportedAsWrongIdentity(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	res, err := h.qa("intruder-token").Record(t.Context(), validVerdict("success"))
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if res.Recorded || res.Reason != ReasonWrongIdentity {
		t.Fatalf("result = %+v, want wrong_identity", res)
	}
}
