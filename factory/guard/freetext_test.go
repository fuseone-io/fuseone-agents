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
		"merge response cut":    func(f *fakeGitHub) { f.mergeFault = faultTruncate },
		"merge outcome unknown": func(f *fakeGitHub) { f.mergeFault, f.readsDown = faultDrop, true },
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

// The same guarantee for the board tools, over the GraphQL fake: titles,
// content, text values and every error message carry the canary.
func TestBoardTools_noResultCarriesFreeTextFromGitHub(t *testing.T) {
	t.Parallel()

	scenarios := map[string]func(f *fakeGraphQL){
		"all valid":          func(*fakeGraphQL) {},
		"wrong project":      func(f *fakeGraphQL) { f.items[testItem].ProjectID = otherProjectID },
		"archived":           func(f *fakeGraphQL) { f.items[testItem].Archived = true },
		"not found":          func(f *fakeGraphQL) { delete(f.items, testItem) },
		"lane unknown":       func(f *fakeGraphQL) { f.items[testItem].Status = "opt-backlog" },
		"in travado":         func(f *fakeGraphQL) { f.items[testItem].Status = "opt-travado" },
		"update errors":      func(f *fakeGraphQL) { f.refuseField = testStatusField },
		"size errors":        func(f *fakeGraphQL) { f.refuseField = testSizeField },
		"update http status": func(f *fakeGraphQL) { f.mutationStatus = 403 },
		"update answer lost": func(f *fakeGraphQL) { f.dropMutation = true },
		"read refused":       func(f *fakeGraphQL) { f.readErrors = true },
		"moved before update": func(f *fakeGraphQL) {
			f.beforeMutation = func(f *fakeGraphQL) { f.items[testItem].Status = "opt-travado" }
		},
	}
	calls := map[string]map[string]any{
		ToolMoveCard:      {"item_id": testItem, "lane": "Preparado"},
		ToolSetCardFields: {"item_id": testItem, "work_type": "bug", "size": "M"},
	}
	for name, mutate := range scenarios {
		for tool, args := range calls {
			t.Run(name+"/"+tool, func(t *testing.T) {
				t.Parallel()
				h := newBoardHarness(t, LaneTriagem)
				h.edit(mutate)
				raw, err := json.Marshal(mustCall(t, h.session(RoleTriage), tool, args))
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

// Every string a move result can carry is a closed reason, the validated
// item id, or a lane from the closed set.
func TestMoveResult_stringFields_areOnlyReasonsIDsAndLanes(t *testing.T) {
	t.Parallel()

	h := newBoardHarness(t, LaneTriagem)
	raw, _ := json.Marshal(h.move(RoleTriage, "Travado"))
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	for key, v := range fields {
		s, ok := v.(string)
		if !ok {
			continue
		}
		_, lane := member(Lanes(), s)
		allowed := (key == "reason" && Reason(s) == ReasonMoved) ||
			(key == "item_id" && s == testItem) ||
			((key == "from" || key == "to") && lane)
		if !allowed {
			t.Fatalf("field %q = %q is not a reason, the item id or a lane", key, s)
		}
	}
}
