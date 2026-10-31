package guard

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
)

// merge is PUT /pulls/{n}/merge: GitHub's own last word. It refuses a moved
// head with 409 and anything the ruleset does not allow with 405, judged at
// this moment rather than when the guard read.
func (f *fakeGitHub) merge(w http.ResponseWriter, r *http.Request, _ ghUser) {
	n, pr := f.pr(r)
	var in struct {
		SHA    string `json:"sha"`
		Method string `json:"merge_method"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	f.attempts = append(f.attempts, mergeCall{PR: n, SHA: in.SHA, Method: in.Method})
	refuse := func(code int) { writeJSON(w, code, map[string]string{"message": canary}) }

	switch {
	case f.mergeError != 0:
		refuse(f.mergeError)
	case pr == nil || pr.State != "open":
		refuse(http.StatusMethodNotAllowed)
	case in.SHA != pr.Head:
		refuse(http.StatusConflict)
	case !f.rulesetAllows(n, pr.Head):
		refuse(http.StatusMethodNotAllowed)
	default:
		pr.State, pr.Merged = "closed", true
		writeJSON(w, http.StatusOK, map[string]any{"sha": mergeSHA, "merged": true, "message": canary})
	}
}

func (f *fakeGitHub) rulesetAllows(n int, head string) bool {
	return f.behind[head] == 0 && f.rulesetReviewed(n, head) &&
		f.rulesetQA(head) && f.rulesetChecks(head)
}

// rulesetReviewed: some reviewer in the ruleset's set whose last review is an
// approval of this head.
func (f *fakeGitHub) rulesetReviewed(n int, head string) bool {
	last := map[string]fakeReview{}
	for _, rv := range f.reviews[n] {
		last[strings.ToLower(rv.Login)] = rv
	}
	for _, login := range f.ruleset.Reviewers {
		rv, ok := last[strings.ToLower(login)]
		if ok && rv.State == "APPROVED" && rv.CommitID == head {
			return true
		}
	}
	return false
}

// rulesetQA: the QA App's own latest factory/qa on this head is a success.
func (f *fakeGitHub) rulesetQA(head string) bool {
	list := f.statuses[head]
	for i := len(list) - 1; i >= 0; i-- {
		if list[i].Context == "factory/qa" && list[i].Creator.ID == f.ruleset.QABotID {
			return list[i].State == "success"
		}
	}
	return false
}

func (f *fakeGitHub) rulesetChecks(head string) bool {
	for _, name := range f.ruleset.Checks {
		i := slices.IndexFunc(f.checks[head], func(c fakeCheck) bool {
			return c.Name == name && c.AppID == f.ruleset.ChecksAppID
		})
		if i < 0 || f.checks[head][i].Conclusion != "success" {
			return false
		}
	}
	return true
}
