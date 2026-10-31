package guard

import (
	"context"
	"strings"
	"time"
)

// reconcileTimeout bounds the re-read after a lost merge response. It runs on
// a context detached from the caller's cancellation: a call that timed out
// waiting for the merge is exactly the one that most needs the re-read.
const reconcileTimeout = 15 * time.Second

// reconcile settles a merge whose request was sent but whose response was
// lost: a cut body, a reset connection, a timeout. GitHub may have merged, so
// a bare error would leave a destructive effect the ledger never learns of.
// It re-reads the PR: merged at the proposed head is merged; anything else,
// including a failed re-read, is outcome_unknown. It never merges again.
func (m *Merger) reconcile(ctx context.Context, res MergeResult) (MergeResult, error) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), reconcileTimeout)
	defer cancel()
	pr, err := m.gh.pullRequest(rctx, res.PR)
	if err == nil && alreadyMerged(m.target, pr, res.HeadSHA) {
		return mergedResult(res, pr.MergeCommitSHA), nil
	}
	res.Reason = ReasonOutcomeUnknown
	return res, nil
}

// alreadyMerged reports a PR in the protected target that GitHub merged with
// exactly this head. A PR merged with any other head carried code this call
// never judged, and is not claimed.
func alreadyMerged(want target, pr pullRequest, head string) bool {
	return pr.Merged && pr.Head.SHA == head &&
		strings.EqualFold(pr.Base.Repo.FullName, want.repo.FullName()) &&
		pr.Base.Ref == want.base
}

// mergedResult marks res merged. The merge commit is kept only if it is a
// SHA, so nothing else GitHub sent can ride along.
func mergedResult(res MergeResult, mergeSHA string) MergeResult {
	res.Merged, res.Reason, res.HTTPStatus = true, ReasonMerged, 0
	if validSHA(mergeSHA) {
		res.MergeSHA = mergeSHA
	}
	return res
}
