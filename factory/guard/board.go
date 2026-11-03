package guard

import (
	"context"
	"fmt"
)

// MoveInput is move_card's argument, as decoded strictly from the call.
type MoveInput struct {
	ItemID string
	Lane   string
}

// MoveResult is structured state only: a closed reason, the validated item
// id, lane names from the closed set, and an HTTP status.
type MoveResult struct {
	Moved      bool   `json:"moved"`
	Reason     Reason `json:"reason"`
	ItemID     string `json:"item_id,omitempty"`
	From       Lane   `json:"from,omitempty"`
	To         Lane   `json:"to,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

// Board changes cards in the one configured project, and only as the calling
// role's row of the transition table allows (#213).
//
// Residual race: GitHub's field updates have no compare-and-set. Between the
// read that judges the move and the one mutation that makes it, a person can
// move the card; the mutation then overwrites their move. The window is one
// round trip, the same shape as merge_if_ready's re-read before its merge,
// and a person who moved a card to Travado sees it leave and moves it back.
type Board struct {
	gh   *Client
	cfg  BoardConfig
	role Role
}

// NewBoard builds the board tools for one role.
func NewBoard(cfg BoardConfig, role Role, gh *Client) *Board {
	return &Board{gh: gh, cfg: cfg, role: role}
}

// Move reads the card, judges it, and changes its Status with exactly one
// mutation. A refusal is a result; an error means the card could not be read
// and nothing was written.
func (b *Board) Move(ctx context.Context, in MoveInput) (MoveResult, error) {
	var res MoveResult
	to, ok := member(Lanes(), in.Lane)
	if !ok || !validItemID(in.ItemID) {
		res.Reason = ReasonInvalidInput
		return res, nil
	}
	res.ItemID, res.To = in.ItemID, to
	item, err := b.gh.readItem(ctx, in.ItemID)
	if err != nil {
		return res, fmt.Errorf("move_card: %w", err)
	}
	if res.From, res.Reason = b.judgeItem(item); res.Reason != "" {
		return res, nil
	}
	if res.Reason = judgeMove(b.role, res.From, to); res.Reason != "" {
		return res, nil
	}
	outcome, code := b.gh.updateField(ctx, fieldUpdate{
		Project: b.cfg.ProjectID, Item: in.ItemID, Field: b.cfg.StatusFieldID, Option: b.cfg.Lanes[to],
	})
	res.Reason, res.HTTPStatus = moveReason(outcome), code
	res.Moved = outcome == writeDone
	return res, nil
}

// judgeItem refuses, in order, an id that is no project item, an item of
// another project, an archived item, and a Status that is unset or no lane
// the configuration names. Otherwise it returns the card's lane.
func (b *Board) judgeItem(item projectItem) (Lane, Reason) {
	switch {
	case !item.Found:
		return "", ReasonItemNotFound
	case item.ProjectID != b.cfg.ProjectID:
		return "", ReasonWrongProject
	case item.Archived:
		return "", ReasonItemArchived
	}
	lane, ok := b.cfg.laneOf(item.Options[b.cfg.StatusFieldID])
	if !ok {
		return "", ReasonLaneUnknown
	}
	return lane, ""
}

func moveReason(o writeOutcome) Reason {
	switch o {
	case writeDone:
		return ReasonMoved
	case writeNotSent:
		return ReasonMoveNotSent
	case writeUnknown:
		// The value is idempotent: a retry that finds the card in the lane
		// answers already_in_lane.
		return ReasonOutcomeUnknown
	}
	return ReasonMoveRefused
}
