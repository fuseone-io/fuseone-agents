package guard

import (
	"context"
	"fmt"
	"net/http"
)

// MergeInput is merge_if_ready's argument: the PR and the head the Merger
// proposed to merge. Frozen at proposal time; the guard re-reads everything
// else when it executes.
type MergeInput struct {
	PR      int    `json:"pr" jsonschema:"the pull request number"`
	HeadSHA string `json:"head_sha" jsonschema:"the full 40-character head commit SHA that was reviewed and tested"`
}

// MergeResult is structured state only: a closed reason, numbers, hex SHAs
// and, for a failing check, the name from this server's own configuration.
type MergeResult struct {
	Merged     bool   `json:"merged"`
	Reason     Reason `json:"reason"`
	PR         int    `json:"pr"`
	HeadSHA    string `json:"head_sha,omitempty"`
	MergeSHA   string `json:"merge_sha,omitempty"`
	Check      string `json:"check,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

// Merger merges a PR only when every signal is on the proposed head and came
// from the actor or app allowed to produce it (NT-013 § Guardrails, layer 5).
type Merger struct {
	gh          *Client
	target      target
	reviewers   []string
	qaBotID     int64
	checks      []string
	checksAppID int64
	method      string
}

// NewMerger builds the merge tool from the Merger instance's configuration.
func NewMerger(cfg Config, gh *Client) *Merger {
	return &Merger{
		gh:          gh,
		target:      target{repo: cfg.Repo, base: cfg.Base},
		reviewers:   cfg.Reviewers,
		qaBotID:     cfg.QABotID,
		checks:      cfg.Checks,
		checksAppID: cfg.ChecksAppID,
		method:      cfg.MergeMethod,
	}
}

// MergeIfReady re-reads GitHub, fails closed on the first condition that does
// not hold, and otherwise merges with sha set to the head, so a push in
// between is refused by GitHub. A refusal is a result; an error means GitHub
// could not be read and nothing was merged.
func (m *Merger) MergeIfReady(ctx context.Context, in MergeInput) (MergeResult, error) {
	res := MergeResult{PR: in.PR}
	if in.PR <= 0 || !validSHA(in.HeadSHA) {
		res.Reason = ReasonInvalidInput
		return res, nil
	}
	res.HeadSHA = in.HeadSHA
	pr, err := m.gh.pullRequest(ctx, in.PR)
	if err != nil {
		return res, fmt.Errorf("merge_if_ready: %w", err)
	}
	if res.Reason = checkPullRequest(m.target, pr, in.HeadSHA); res.Reason != "" {
		return res, nil
	}
	sig, err := m.gh.readSignals(ctx, in.PR, m.target.base, in.HeadSHA)
	if err != nil {
		return res, fmt.Errorf("merge_if_ready: %w", err)
	}
	if res.Reason, res.Check = m.judge(pr, sig, in.HeadSHA); res.Reason != "" {
		return res, nil
	}
	return m.merge(ctx, res)
}

// judge applies the remaining conditions in order, returning the first that
// fails and, for a check, which one.
func (m *Merger) judge(pr pullRequest, sig signals, head string) (Reason, string) {
	if r := checkReviews(m.reviewers, sig.reviews, head); r != "" {
		return r, ""
	}
	if r := checkQA(m.qaBotID, sig.statuses); r != "" {
		return r, ""
	}
	if r, name := checkRuns(m.checks, m.checksAppID, sig.checks, head); r != "" {
		return r, name
	}
	return checkFreshness(pr, sig.behindBy), ""
}

// merge makes the one merge call. GitHub's refusal is reported as it came,
// by status code only, and never retried: a refusal here means a signal
// changed after the read, and only a fresh proposal may judge it again.
func (m *Merger) merge(ctx context.Context, res MergeResult) (MergeResult, error) {
	var out struct {
		SHA    string `json:"sha"`
		Merged bool   `json:"merged"`
	}
	body := map[string]string{"sha": res.HeadSHA, "merge_method": m.method}
	code, err := m.gh.send(ctx, apiRequest{
		Method: http.MethodPut, Path: m.gh.repoPath("/pulls/%d/merge", res.PR), Body: body, Out: &out,
	})
	if err != nil {
		return res, fmt.Errorf("merge_if_ready: %w", err)
	}
	switch {
	case code == http.StatusOK && out.Merged:
		res.Merged, res.Reason = true, ReasonMerged
		if validSHA(out.SHA) {
			res.MergeSHA = out.SHA
		}
	case code == http.StatusConflict:
		res.Reason, res.HTTPStatus = ReasonHeadMoved, code
	default:
		res.Reason, res.HTTPStatus = ReasonMergeRefused, code
	}
	return res, nil
}
