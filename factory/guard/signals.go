package guard

import (
	"cmp"
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
)

// review, commitStatus and checkRun decode only the fields a decision reads.
// Bodies, descriptions, check output and app names are never decoded.
type review struct {
	ID       int64  `json:"id"`
	State    string `json:"state"`
	CommitID string `json:"commit_id"`
	User     struct {
		Login string `json:"login"`
	} `json:"user"`
}

type commitStatus struct {
	ID      int64         `json:"id"`
	State   string        `json:"state"`
	Context string        `json:"context"`
	Creator statusCreator `json:"creator"`
}

type checkRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HeadSHA    string `json:"head_sha"`
	App        struct {
		ID int64 `json:"id"`
	} `json:"app"`
}

// signals is everything merge_if_ready judges, read fresh when it executes.
type signals struct {
	reviews  []review
	statuses []commitStatus
	checks   []checkRun
	behindBy int
}

func (c *Client) readSignals(ctx context.Context, pr int, base, head string) (signals, error) {
	var s signals
	var err error
	if s.reviews, err = listAll(ctx, c, c.repoPath("/pulls/%d/reviews", pr), plainList[review]); err != nil {
		return s, err
	}
	if s.statuses, err = listAll(ctx, c, c.repoPath("/commits/%s/statuses", head), plainList[commitStatus]); err != nil {
		return s, err
	}
	if s.checks, err = listAll(ctx, c, c.repoPath("/commits/%s/check-runs", head), checkRunList); err != nil {
		return s, err
	}
	var cmp struct {
		BehindBy int `json:"behind_by"`
	}
	err = c.get(ctx, c.repoPath("/compare/%s...%s", url.PathEscape(base), head), &cmp)
	s.behindBy = cmp.BehindBy
	return s, err
}

func checkRunList(raw json.RawMessage) ([]checkRun, error) {
	var page struct {
		CheckRuns []checkRun `json:"check_runs"`
	}
	err := json.Unmarshal(raw, &page)
	return page.CheckRuns, err
}

// checkReviews wants an approval of this head from an authorised reviewer,
// and no authorised reviewer currently requesting changes. Each reviewer's
// standing is their latest review that decides something; a comment does not.
func checkReviews(authorised []string, reviews []review, head string) Reason {
	latest := map[string]review{}
	for _, r := range slices.SortedFunc(slices.Values(reviews), byReviewID) {
		if r.State != "COMMENTED" && isAuthorised(authorised, r.User.Login) {
			latest[strings.ToLower(r.User.Login)] = r
		}
	}
	reason := ReasonReviewMissing
	for _, r := range latest {
		switch {
		case r.State == "CHANGES_REQUESTED":
			return ReasonChangesRequested
		case r.State == "APPROVED" && r.CommitID == head:
			reason = ""
		case r.State == "APPROVED" && reason == ReasonReviewMissing:
			reason = ReasonReviewStale
		}
	}
	return reason
}

func byReviewID(a, b review) int { return cmp.Compare(a.ID, b.ID) }

// isAuthorised compares logins case-insensitively, as GitHub does.
func isAuthorised(authorised []string, login string) bool {
	return slices.ContainsFunc(authorised, func(a string) bool { return strings.EqualFold(a, login) })
}

// checkQA judges the latest factory/qa status on the head, whoever set it. It
// must come from the QA App's bot and say success; a later status from anyone
// else makes the signal ambiguous, and ambiguity is a refusal.
func checkQA(qaBotID int64, statuses []commitStatus) Reason {
	var latest *commitStatus
	for i := range statuses {
		s := &statuses[i]
		if s.Context == QAContext && (latest == nil || s.ID > latest.ID) {
			latest = s
		}
	}
	switch {
	case latest == nil:
		return ReasonQAMissing
	case latest.Creator.ID != qaBotID || latest.Creator.Type != "Bot":
		return ReasonQAUntrustedCreator
	case latest.State != "success":
		return ReasonQANotSuccess
	}
	return ""
}

// checkRuns wants every expected check, as emitted by the expected app on the
// head, completed with success. Runs by any other app are ignored entirely:
// a name proves nothing, since anyone with checks write can publish one.
// It returns the configured name of the first check that fails.
func checkRuns(expected []string, appID int64, runs []checkRun, head string) (Reason, string) {
	for _, name := range expected {
		run, ok := latestRun(runs, name, appID, head)
		if !ok {
			return ReasonCheckMissing, name
		}
		if run.Status != "completed" || run.Conclusion != "success" {
			return ReasonCheckNotSuccess, name
		}
	}
	return "", ""
}

func latestRun(runs []checkRun, name string, appID int64, head string) (checkRun, bool) {
	var found checkRun
	ok := false
	for _, r := range runs {
		if r.Name == name && r.App.ID == appID && r.HeadSHA == head && (!ok || r.ID > found.ID) {
			found, ok = r, true
		}
	}
	return found, ok
}

// checkFreshness enforces strict status checks: the head must contain the
// current base, and GitHub must have found it mergeable.
func checkFreshness(pr pullRequest, behindBy int) Reason {
	switch {
	case behindBy > 0:
		return ReasonBaseBehind
	case pr.Mergeable == nil:
		return ReasonMergeabilityUnknown
	case !*pr.Mergeable:
		return ReasonNotMergeable
	}
	return ""
}
