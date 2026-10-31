package guard

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode"
)

// maxDescription is GitHub's limit on a commit status description; a longer
// one is refused outright, losing the verdict.
const maxDescription = 140

// VerdictInput is record_qa_verdict's argument.
type VerdictInput struct {
	PR        int    `json:"pr" jsonschema:"the pull request number"`
	HeadSHA   string `json:"head_sha" jsonschema:"the full 40-character head commit SHA the verdict is about"`
	Verdict   string `json:"verdict" jsonschema:"success or failure"`
	Summary   string `json:"summary" jsonschema:"one line shown as the status description; cut at 140 characters"`
	CommentID int64  `json:"comment_id,omitempty" jsonschema:"id of the PR comment holding the full verdict; the status links to it"`
}

// VerdictResult is structured state only.
type VerdictResult struct {
	Recorded bool   `json:"recorded"`
	Reason   Reason `json:"reason"`
	PR       int    `json:"pr"`
	HeadSHA  string `json:"head_sha,omitempty"`
	State    string `json:"state,omitempty"`
}

// QA records the QA role's verdict as the factory/qa commit status.
type QA struct {
	gh     *Client
	target target
	botID  int64
	webURL string
}

// NewQA builds the QA tool. gh must carry the QA App's installation token:
// the ruleset accepts factory/qa only from that App (#207, #208).
func NewQA(cfg Config, gh *Client) *QA {
	return &QA{
		gh:     gh,
		target: target{repo: cfg.Repo, base: cfg.Base},
		botID:  cfg.QABotID,
		webURL: strings.TrimRight(cfg.WebURL, "/"),
	}
}

// Record sets factory/qa on exactly in.HeadSHA, refusing if that is no longer
// the PR's head. A refusal is a result, not an error; an error means GitHub
// could not be read and nothing was set.
func (q *QA) Record(ctx context.Context, in VerdictInput) (VerdictResult, error) {
	res := VerdictResult{PR: in.PR}
	if !wellFormedVerdict(in) {
		res.Reason = ReasonInvalidInput
		return res, nil
	}
	res.HeadSHA = in.HeadSHA
	pr, err := q.gh.pullRequest(ctx, in.PR)
	if err != nil {
		return res, fmt.Errorf("record_qa_verdict: %w", err)
	}
	if reason := checkPullRequest(q.target, pr, in.HeadSHA); reason != "" {
		res.Reason = reason
		return res, nil
	}
	creator, code, err := q.gh.createStatus(ctx, in.HeadSHA, statusRequest{
		State:       in.Verdict,
		Context:     QAContext,
		Description: description(in.Summary),
		TargetURL:   q.targetURL(in),
	})
	if err != nil {
		return res, fmt.Errorf("record_qa_verdict: %w", err)
	}
	res.Reason = q.judgeCreated(code, creator)
	res.Recorded = res.Reason == ReasonRecorded
	if res.Recorded {
		res.State = in.Verdict
	}
	return res, nil
}

// judgeCreated confirms the status was created, and created by the QA App:
// one created under any other identity counts for nothing downstream.
func (q *QA) judgeCreated(code int, creator statusCreator) Reason {
	if code != http.StatusCreated {
		return ReasonStatusRefused
	}
	if creator.ID != q.botID || creator.Type != "Bot" {
		return ReasonWrongIdentity
	}
	return ReasonRecorded
}

func wellFormedVerdict(in VerdictInput) bool {
	return in.PR > 0 && validSHA(in.HeadSHA) && in.CommentID >= 0 &&
		(in.Verdict == "success" || in.Verdict == "failure") &&
		strings.TrimSpace(in.Summary) != ""
}

// description folds the summary onto one line and cuts it to GitHub's limit,
// counting characters rather than bytes.
func description(summary string) string {
	line := strings.Join(strings.FieldsFunc(summary, unicode.IsSpace), " ")
	runes := []rune(line)
	if len(runes) > maxDescription {
		runes = runes[:maxDescription]
	}
	return string(runes)
}

// targetURL links the status to the verdict comment, or to the PR without
// one. Built from configuration and integers, never from a GitHub string.
func (q *QA) targetURL(in VerdictInput) string {
	url := fmt.Sprintf("%s/%s/pull/%d", q.webURL, q.target.repo.FullName(), in.PR)
	if in.CommentID > 0 {
		url += fmt.Sprintf("#issuecomment-%d", in.CommentID)
	}
	return url
}

type statusRequest struct {
	State       string `json:"state"`
	Context     string `json:"context"`
	Description string `json:"description"`
	TargetURL   string `json:"target_url"`
}

// statusCreator is who GitHub says created a status. For an App it is the
// App's bot user: statuses carry no app id, so the bot's user id is what
// identifies the QA App on a status.
type statusCreator struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

func (c *Client) createStatus(ctx context.Context, sha string, req statusRequest) (statusCreator, int, error) {
	var out struct {
		Creator statusCreator `json:"creator"`
	}
	code, err := c.send(ctx, http.MethodPost, c.repoPath("/statuses/%s", sha), req, &out)
	return out.Creator, code, err
}
