package guard

import (
	"encoding/json"
	"strings"
	"testing"
)

// The fake plants a canary in every free-text field it serves and in every
// refusal message. Whatever a tool answers, on success, on refusal, on a
// merge GitHub turned down, or on an error, the canary must not come back:
// the Merger's run reads these results, and free text would taint it.
func TestTools_noResultCarriesFreeTextFromGitHub(t *testing.T) {
	t.Parallel()

	scenarios := map[string]func(f *fakeGitHub){
		"all valid":             func(*fakeGitHub) {},
		"head moved":            func(f *fakeGitHub) { f.prs[testPR].Head = headB },
		"wrong base":            func(f *fakeGitHub) { f.prs[testPR].BaseRef = "develop" },
		"review stale":          func(f *fakeGitHub) { f.reviews[testPR][0].CommitID = headB },
		"qa from someone else":  func(f *fakeGitHub) { f.statuses[headA][0].Creator = intruder },
		"check failed":          func(f *fakeGitHub) { f.checks[headA][0].Conclusion = "failure" },
		"base behind":           func(f *fakeGitHub) { f.behind[headA] = 2 },
		"merge refused":         func(f *fakeGitHub) { f.mergeError = 405 },
		"push before the merge": func(f *fakeGitHub) { f.beforeMerge = func(f *fakeGitHub) { f.prs[testPR].Head = headB } },
		"pr unknown":            func(f *fakeGitHub) { delete(f.prs, testPR) },
		"repository redirects":  func(f *fakeGitHub) { f.renamed = true },
	}
	calls := map[string]struct {
		role  Role
		token string
		args  map[string]any
	}{
		ToolMergeIfReady: {RoleMerger, "merger-token", map[string]any{"pr": testPR, "head_sha": headA}},
		ToolRecordQAVerdict: {RoleQA, "qa-token", map[string]any{
			"pr": testPR, "head_sha": headA, "verdict": "success", "summary": "ok"}},
	}
	for name, mutate := range scenarios {
		for tool, call := range calls {
			t.Run(name+"/"+tool, func(t *testing.T) {
				t.Parallel()
				h := newHarness(t)
				h.edit(mutate)
				res := h.call(h.session(call.role, call.token), tool, call.args)
				raw, err := json.Marshal(res)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(raw), canary) || strings.Contains(string(raw), "canary") {
					t.Fatalf("result carries GitHub's free text: %s", raw)
				}
			})
		}
	}
}

// The canary check proves absence; this proves the shape. Every string a
// merge result can carry is a closed reason, a hex SHA, or a configured name.
func TestMergeResult_stringFields_areOnlyReasonsSHAsAndConfiguredNames(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.edit(func(f *fakeGitHub) { f.checks[headA][1].Conclusion = "failure" })
	res := h.mergeIfReady(headA)
	raw, _ := json.Marshal(res)
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	for key, v := range fields {
		s, ok := v.(string)
		if !ok {
			continue
		}
		allowed := (key == "reason" && Reason(s) == ReasonCheckNotSuccess) ||
			(key == "head_sha" && validSHA(s)) ||
			(key == "check" && s == "postgres")
		if !allowed {
			t.Fatalf("field %q = %q is not a reason, a SHA or a configured name", key, s)
		}
	}
}
