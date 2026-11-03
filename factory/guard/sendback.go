package guard

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The factory's sub-state labels on a pull request (factory/LANES.md).
const (
	LabelReadyToMerge     = "factory:ready-to-merge"
	LabelNeedsReview      = "factory:needs-review"
	LabelChangesRequested = "factory:changes-requested"
	LabelNeedsQA          = "factory:needs-qa"
)

// SendBackInput is send_back's argument: the pull request, the head the
// Merger judged, and the closed reason it was not merged.
type SendBackInput struct {
	PR      int    `json:"pr" jsonschema:"the pull request number"`
	HeadSHA string `json:"head_sha" jsonschema:"the full 40-character head commit SHA the Merger judged"`
	Reason  string `json:"reason" jsonschema:"the reason merge_if_ready or list_ready_prs gave"`
}

// SendBackResult is structured state only: closed reasons, numbers, label
// names from this package's constants, and the id GitHub gave the comment.
type SendBackResult struct {
	SentBack   bool   `json:"sent_back"`
	Reason     Reason `json:"reason"`
	Cause      Reason `json:"cause,omitempty"`
	PR         int    `json:"pr"`
	Added      string `json:"added,omitempty"`
	Removed    string `json:"removed,omitempty"`
	CommentID  int64  `json:"comment_id,omitempty"`
	Travado    bool   `json:"travado"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

// sendBackRoute is where a refusal sends the pull request: to the role whose
// signal is missing (a new sub-state label), or, when only a person can act,
// nowhere (the label stays and the Merger moves the card to Travado).
type sendBackRoute struct {
	to     string
	action string
}

// routeFor is the closed table (#199). A reason it does not name is not one
// send_back accepts: a transient one (check_missing, mergeability_unknown)
// clears by itself, and one about the target (wrong_base) is no factory PR.
func routeFor(cause Reason) (sendBackRoute, bool) {
	switch cause {
	case ReasonHeadMoved, ReasonReviewMissing, ReasonReviewStale:
		return sendBackRoute{LabelNeedsReview, "The Reviewer reviews the current head."}, true
	case ReasonChangesRequested:
		return sendBackRoute{LabelChangesRequested, "The Developer answers the review."}, true
	case ReasonQANotSuccess:
		return sendBackRoute{LabelChangesRequested, "The Developer answers QA's verdict."}, true
	case ReasonCheckNotSuccess:
		return sendBackRoute{LabelChangesRequested, "The Developer fixes the failing check."}, true
	case ReasonQAMissing, ReasonQAUntrustedCreator:
		return sendBackRoute{LabelNeedsQA, "QA records its verdict on the current head."}, true
	case ReasonBaseBehind:
		return sendBackRoute{"", "Please update the branch with the base, then return the card."}, true
	case ReasonNotMergeable:
		return sendBackRoute{"", "Please resolve the conflict with the base, then return the card."}, true
	case ReasonMergeRefused, ReasonOutcomeUnknown:
		return sendBackRoute{"", "Please check why GitHub did not merge it (do the ruleset's required " +
			"checks and FACTORY_CHECKS differ?), then return the card."}, true
	}
	return sendBackRoute{}, false
}

// SendBack writes a refusal back to a pull request the Merger did not merge.
type SendBack struct {
	gh     *Client
	target target
	merger *Merger // re-judges the head, and holds the refusals it observed
}

// NewMergerTools builds merge_if_ready and send_back sharing one record of
// the refusals the server observed, so send_back can prove the causes a
// re-read cannot rebuild.
func NewMergerTools(cfg Config, gh *Client, clock Clock) (*Merger, *SendBack) {
	m := NewMerger(cfg, gh)
	m.refusals = newRefusals(clock)
	return m, &SendBack{gh: gh, target: target{repo: cfg.Repo, base: cfg.Base}, merger: m}
}

// SendBack writes nothing unless the server itself observes the cause on
// the given head: a cause a re-read can rebuild is judged again now, exactly
// as merge_if_ready would, and merge_refused or outcome_unknown must be one
// this process's merge_if_ready returned for that head. The caller's reason
// and SHA name what to check; they are never proof.
//
// It then posts the closed-text comment first and moves the label after:
// add the new one, then remove ready-to-merge. A failure between the writes
// leaves the PR still ready-to-merge, so the next send_back repairs it; the
// one cost of a retry is a repeated comment.
func (s *SendBack) SendBack(ctx context.Context, in SendBackInput) (SendBackResult, error) {
	res := SendBackResult{PR: in.PR}
	route, ok := routeFor(Reason(in.Reason))
	if in.PR <= 0 || !validSHA(in.HeadSHA) || !ok {
		res.Reason = ReasonInvalidInput
		return res, nil
	}
	res.Cause = Reason(in.Reason)
	pr, err := s.gh.pullRequest(ctx, in.PR)
	if err != nil {
		return res, fmt.Errorf("send_back: %w", err)
	}
	if res.Reason = s.judge(pr); res.Reason != "" {
		return res, nil
	}
	var taken time.Time
	if res.Reason, taken, err = s.observe(ctx, pr, in.PR, in.HeadSHA, res.Cause); err != nil || res.Reason != "" {
		return res, err
	}
	res.Travado = route.to == ""
	if res.CommentID, res.Reason, res.HTTPStatus = s.comment(ctx, in.PR, commentBody(res.Cause, in.HeadSHA, route)); res.Reason != "" {
		if !taken.IsZero() && (res.Reason == ReasonCommentNotSent || res.Reason == ReasonCommentRefused) {
			// Known not written: the refusal still proves a retry.
			s.merger.refusals.restore(in.PR, in.HeadSHA, res.Cause, taken)
		}
		return res, nil
	}
	if route.to != "" {
		if res.Reason, res.HTTPStatus = s.relabel(ctx, in.PR, route.to); res.Reason != "" {
			return res, nil
		}
		res.Added, res.Removed = route.to, LabelReadyToMerge
	}
	res.SentBack, res.Reason = true, ReasonSentBack
	return res, nil
}

func (s *SendBack) judge(pr pullRequest) Reason {
	switch {
	case !strings.EqualFold(pr.Base.Repo.FullName, s.target.repo.FullName()):
		return ReasonWrongRepository
	case pr.Base.Ref != s.target.base:
		return ReasonWrongBase
	case pr.State != "open" || pr.Merged:
		return ReasonPRNotOpen
	case !pr.hasLabel(LabelReadyToMerge):
		return ReasonNotReadyToMerge
	}
	return ""
}

// observe answers "" when the server sees cause on head right now, and
// cause_not_observed otherwise: a head that moved, a signal repaired in the
// meantime, a cause the caller made up. For merge_refused and outcome_unknown
// it spends the refusal merge_if_ready recorded, and reports when that was
// observed so a write known not to have happened can give it back.
func (s *SendBack) observe(ctx context.Context, pr pullRequest, number int, head string, cause Reason) (Reason, time.Time, error) {
	if unrebuildable(cause) {
		if pr.Head.SHA != head {
			return ReasonCauseNotObserved, time.Time{}, nil
		}
		at, ok := s.merger.refusals.take(number, head, cause)
		if !ok {
			return ReasonCauseNotObserved, time.Time{}, nil
		}
		return "", at, nil
	}
	now, _, err := s.merger.prejudge(ctx, number, head)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("send_back: %w", err)
	}
	if now != cause {
		return ReasonCauseNotObserved, time.Time{}, nil
	}
	return "", time.Time{}, nil
}

// commentBody is closed text: the reason, the judged head and the action,
// each from this package. A card going to Travado ends with its return line.
func commentBody(cause Reason, head string, route sendBackRoute) string {
	body := fmt.Sprintf("Merger: not merged: %s on %s.\n\n%s", cause, head[:12], route.action)
	if route.to == "" {
		body += "\n\nReturn to: Doing"
	}
	return body
}

func (s *SendBack) comment(ctx context.Context, pr int, body string) (int64, Reason, int) {
	var out struct {
		ID int64 `json:"id"`
	}
	code, err := s.gh.send(ctx, apiRequest{
		Method: http.MethodPost, Path: s.gh.repoPath("/issues/%d/comments", pr),
		Body: map[string]string{"body": body}, Out: &out,
	})
	switch {
	case errors.Is(err, ErrNotSent):
		return 0, ReasonCommentNotSent, 0
	case err != nil:
		return 0, ReasonOutcomeUnknown, 0
	case code != http.StatusCreated || out.ID <= 0:
		return 0, ReasonCommentRefused, code
	}
	return out.ID, "", 0
}

// relabel adds the sub-state label, then removes ready-to-merge. A remove
// answered 404 means the label is already gone: done.
func (s *SendBack) relabel(ctx context.Context, pr int, to string) (Reason, int) {
	code, err := s.gh.send(ctx, apiRequest{
		Method: http.MethodPost, Path: s.gh.repoPath("/issues/%d/labels", pr),
		Body: map[string][]string{"labels": {to}},
	})
	if r := labelReason(err, code, http.StatusOK); r != "" {
		return r, code
	}
	code, err = s.gh.send(ctx, apiRequest{
		Method: http.MethodDelete,
		Path:   s.gh.repoPath("/issues/%d/labels/%s", pr, url.PathEscape(LabelReadyToMerge)),
	})
	if code == http.StatusNotFound {
		return "", 0
	}
	if r := labelReason(err, code, http.StatusOK); r != "" {
		return r, code
	}
	return "", 0
}

func labelReason(err error, code, want int) Reason {
	switch {
	case errors.Is(err, ErrNotSent):
		return ReasonLabelNotSent
	case err != nil:
		return ReasonOutcomeUnknown
	case code != want:
		return ReasonLabelRefused
	}
	return ""
}
