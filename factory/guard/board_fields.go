package guard

import (
	"context"
	"fmt"
)

// FieldsInput is set_card_fields' argument, as decoded strictly from the call.
type FieldsInput struct {
	ItemID, WorkType, Size string
}

// FieldsResult is structured state only. AlreadySet and Changed name the
// fields by the closed CardField set, so a retry after a partial update shows
// what converged without carrying anything GitHub wrote.
type FieldsResult struct {
	Updated    bool        `json:"updated"`
	Reason     Reason      `json:"reason"`
	ItemID     string      `json:"item_id,omitempty"`
	WorkType   WorkType    `json:"work_type,omitempty"`
	Size       Size        `json:"size,omitempty"`
	AlreadySet []CardField `json:"already_set,omitempty"`
	Changed    []CardField `json:"changed,omitempty"`
	HTTPStatus int         `json:"http_status,omitempty"`
}

// SetFields sets a card's Work type and Size, and nothing else, and only on a
// card in Triagem. It is idempotent: a field
// already holding the value is reported as already set and not written, so a
// retry after a partial failure writes only what is still missing. Each
// remaining field is one mutation, in a fixed order; the first that does not
// apply stops the call.
func (b *Board) SetFields(ctx context.Context, in FieldsInput) (FieldsResult, error) {
	var res FieldsResult
	wt, okType := member(WorkTypes(), in.WorkType)
	size, okSize := member(Sizes(), in.Size)
	if !okType || !okSize || !validItemID(in.ItemID) {
		res.Reason = ReasonInvalidInput
		return res, nil
	}
	res.ItemID, res.WorkType, res.Size = in.ItemID, wt, size
	item, err := b.gh.readItem(ctx, in.ItemID)
	if err != nil {
		return res, fmt.Errorf("set_card_fields: %w", err)
	}
	if res.Reason = b.judgeFieldsCard(item); res.Reason != "" {
		return res, nil
	}
	for _, w := range b.wanted(in.ItemID, wt, size) {
		if item.Options[w.update.Field] == w.update.Option {
			res.AlreadySet = append(res.AlreadySet, w.field)
			continue
		}
		outcome, code := b.gh.updateField(ctx, w.update)
		if outcome != writeDone {
			res.Reason, res.HTTPStatus = fieldsReason(outcome), code
			return res, nil
		}
		res.Changed = append(res.Changed, w.field)
	}
	res.Updated = len(res.Changed) > 0
	res.Reason = ReasonUpdated
	if !res.Updated {
		res.Reason = ReasonAlreadySet
	}
	return res, nil
}

// judgeFieldsCard applies move_card's item checks, then allows only a card
// in Triagem. Work type and Size are the classification triage decides on
// the way to Preparado (LANES.md); once a card has left Triagem they are
// locked, so no agent rewrites them during construction or after it. Travado
// and Done keep their own reasons.
func (b *Board) judgeFieldsCard(item projectItem) Reason {
	lane, reason := b.judgeItem(item)
	switch {
	case reason != "":
		return reason
	case lane == LaneTravado:
		return ReasonTravadoIsHuman
	case lane == LaneDone:
		return ReasonDoneIsFinal
	case lane != LaneTriagem:
		return ReasonFieldsLocked
	}
	return ""
}

type wantedField struct {
	field  CardField
	update fieldUpdate
}

// wanted is the two writes, each with the configured project and field.
func (b *Board) wanted(item string, wt WorkType, size Size) []wantedField {
	return []wantedField{
		{FieldWorkType, fieldUpdate{Project: b.cfg.ProjectID, Item: item, Field: b.cfg.WorkTypeFieldID, Option: b.cfg.WorkTypes[wt]}},
		{FieldSize, fieldUpdate{Project: b.cfg.ProjectID, Item: item, Field: b.cfg.SizeFieldID, Option: b.cfg.Sizes[size]}},
	}
}

func fieldsReason(o writeOutcome) Reason {
	switch o {
	case writeNotSent:
		return ReasonUpdateNotSent
	case writeUnknown:
		return ReasonOutcomeUnknown
	}
	return ReasonUpdateRefused
}
