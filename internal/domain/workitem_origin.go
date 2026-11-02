package domain

import (
	"errors"
	"fmt"
	"strings"
)

/*
A run's inherited work item: the card a run is attributed to because of how
it was opened, not because it claimed anything (#204, slice 3).

Attribution only. An inheritance never holds the card, is never written into
run_work_items — the table the claim is judged against — and never blocks a
claim: a run that inherited a card may claim it, or another, exactly as if it
had inherited nothing. What it buys is the trail: a Reviewer an event opened
from a Developer's run shows on the Developer's card, and a run a GitHub
delivery opened shows on the issue or pull request the delivery was about.

Set by the platform alone — the event dispatcher from the source run's own
record, the GitHub door from the structural fields of a signed delivery — and
sealed on run_started. Never parsed from anything a model wrote or from the
free text of a body.
*/

// WorkItemOriginSource is how a run came by its inherited work item.
type WorkItemOriginSource string

const (
	// WorkItemFromEvent is a run an event opened, attributed to the card its
	// source run claimed or itself inherited.
	WorkItemFromEvent WorkItemOriginSource = "event"
	// WorkItemFromGitHubDelivery is a run a signed GitHub delivery opened,
	// attributed to the issue or pull request the delivery names.
	WorkItemFromGitHubDelivery WorkItemOriginSource = "github_delivery"
)

// The links an inheritance puts a run on a trail by, beside the claim and the
// alias: the inheritance's own source, so the trail says how the run came.
const (
	WorkItemLinkEvent          = WorkItemLink(WorkItemFromEvent)
	WorkItemLinkGitHubDelivery = WorkItemLink(WorkItemFromGitHubDelivery)
)

// RunWorkItem is the inherited work item sealed on a run's opening step.
type RunWorkItem struct {
	Ref    WorkItemRef          `json:"ref"`
	Source WorkItemOriginSource `json:"source"`
	// FromRun is the run an event-opened run inherited from: the source
	// itself, never the root of the chain. Absent for a delivery.
	FromRun RunID `json:"from_run,omitempty"`
}

// ErrRunWorkItemInvalid is an inherited work item the ledger cannot record.
var ErrRunWorkItemInvalid = errors.New("inherited work item is not valid")

// Validate refuses an inheritance whose reference is not canonical, whose
// source is unknown, or whose source and FromRun disagree: an event names the
// run it came from, a delivery names none and is always a GitHub reference.
func (w RunWorkItem) Validate() error {
	if ref, ok := ParseWorkItemRef(string(w.Ref)); !ok || ref != w.Ref {
		return fmt.Errorf("%w: reference %q", ErrRunWorkItemInvalid, w.Ref)
	}
	switch w.Source {
	case WorkItemFromEvent:
		if w.FromRun == "" {
			return fmt.Errorf("%w: an event names the run it came from", ErrRunWorkItemInvalid)
		}
	case WorkItemFromGitHubDelivery:
		if w.FromRun != "" {
			return fmt.Errorf("%w: a delivery comes from no run", ErrRunWorkItemInvalid)
		}
		if !strings.HasPrefix(string(w.Ref), "github:") {
			return fmt.Errorf("%w: a delivery names an issue or pull request", ErrRunWorkItemInvalid)
		}
	default:
		return fmt.Errorf("%w: source %q", ErrRunWorkItemInvalid, w.Source)
	}
	return nil
}

/*
InheritWorkItem is what a run an event opened from source inherits.

The source's own claim wins: the run that claimed a card did its work on it.
Otherwise what the source itself inherited passes on, so a chain of events
stays on the card its first run was about. Either way the listener's
inheritance is an event from source, the run it came from — never a copy of
the source's own provenance. An inheritance that would not validate passes
nothing on: attribution is never worth a run that does not open.
*/
func InheritWorkItem(source RunID, claimed WorkItemRef, inherited *RunWorkItem) *RunWorkItem {
	ref := claimed
	if ref == "" && inherited != nil && inherited.Validate() == nil {
		ref = inherited.Ref
	}
	if ref == "" {
		return nil
	}
	out := &RunWorkItem{Ref: ref, Source: WorkItemFromEvent, FromRun: source}
	if out.Validate() != nil {
		return nil
	}
	return out
}
