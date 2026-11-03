package guard

import (
	"context"
	"fmt"
)

// CloseInput is close_merged's argument: the merged pull request.
type CloseInput struct {
	PR int `json:"pr" jsonschema:"the merged pull request number list_ready_prs listed under merged"`
}

// CloseResult is structured state only: a closed reason, the pull request,
// the card's item id, whether it was moved, and an HTTP status.
type CloseResult struct {
	Closed     bool   `json:"closed"`
	Reason     Reason `json:"reason"`
	PR         int    `json:"pr"`
	ItemID     string `json:"item_id,omitempty"`
	Moved      bool   `json:"moved"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

// CloseMerged finishes a merged pull request still labelled ready-to-merge:
// its card to Done, then the label dropped. It is the only way the Merger
// moves a card to Done, and the repair for a run that ended between its merge
// and that move: list_ready_prs keeps listing the pull request under merged
// until the label is gone, and the label goes only once the card reached
// Done, so every failure leaves it for the next run. Calling it again on a
// finished pull request writes nothing.
//
// It judges from the same listing list_ready_prs answers, read again now:
// the caller's number names what to check, and is never proof it merged.
func (l *ReadyLister) CloseMerged(ctx context.Context, in CloseInput) (CloseResult, error) {
	res := CloseResult{PR: in.PR}
	if in.PR <= 0 {
		res.Reason = ReasonInvalidInput
		return res, nil
	}
	node, found, err := l.labelled(ctx, in.PR)
	switch {
	case err != nil:
		return res, fmt.Errorf("close_merged: %w", err)
	case !found:
		res.Reason = ReasonNotReadyToMerge
		return res, nil
	case node.State != "MERGED":
		res.Reason = ReasonPRNotMerged
		return res, nil
	}
	m, ok, err := l.closable(ctx, node)
	if err != nil {
		return res, fmt.Errorf("close_merged: %w", err)
	}
	if !ok {
		res.Reason = ReasonCardNotClosable
		return res, nil
	}
	res.ItemID = m.ItemID
	if m.Lane == LaneDoing {
		if res.Reason, res.HTTPStatus = l.toDone(ctx, m.ItemID); res.Reason != "" {
			return res, nil
		}
		res.Moved = true
	}
	if res.Reason, res.HTTPStatus = l.gh.dropReadyLabel(ctx, in.PR); res.Reason != "" {
		return res, nil
	}
	res.Closed, res.Reason = true, ReasonClosedOut
	return res, nil
}

// labelled finds the pull request in the listing: open or merged, on the
// base, still labelled ready-to-merge.
func (l *ReadyLister) labelled(ctx context.Context, pr int) (readyNode, bool, error) {
	nodes, err := l.gh.readyPullRequests(ctx, l.base)
	if err != nil {
		return readyNode{}, false, err
	}
	for _, n := range nodes {
		if n.Number == pr {
			return n, true, nil
		}
	}
	return readyNode{}, false, nil
}

// toDone moves the card with exactly one mutation, outside move_card's
// table on purpose: Done is reached only here, for a merged pull request.
func (l *ReadyLister) toDone(ctx context.Context, item string) (Reason, int) {
	cfg := l.board.cfg
	outcome, code := l.gh.updateField(ctx, fieldUpdate{
		Project: cfg.ProjectID, Item: item, Field: cfg.StatusFieldID, Option: cfg.Lanes[LaneDone],
	})
	if outcome == writeDone {
		return "", 0
	}
	return moveReason(outcome), code
}
