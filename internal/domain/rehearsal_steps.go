package domain

import (
	"errors"
	"fmt"
)

// The steps of a live rehearsal (NT-018 phase 4). Every reader knows them one release
// before anything writes them, as work_item_linked was (#237): a replica that did not
// would claim, or fail to claim, the wrong run.
const (
	// StepWorldAwaited is a rehearsal waiting for its author to answer a tool call for
	// the world. The run is not claimable until the answer arrives.
	StepWorldAwaited StepKind = "world_awaited"
	// StepWorldSupplied is the author's answer, kept in the content store and named here
	// by reference and digest, never by its text.
	StepWorldSupplied StepKind = "world_supplied"
	// StepRehearsalDecided is the author's decision where the Gate held an action in a
	// rehearsal. It is never approval_decided: no count, report or audit of real
	// approvals may mistake it for one, and it proves nothing outside the rehearsal.
	StepRehearsalDecided StepKind = "rehearsal_decided"
)

// ErrRehearsalStepInvalid is a rehearsal step that does not say what it must.
var ErrRehearsalStepInvalid = errors.New("rehearsal step is not valid")

// WorldAwaitedPayload names the call the rehearsal waits on.
type WorldAwaitedPayload struct {
	Tool ToolID `json:"tool"`
	// CallSeq is the tool_called step this wait answers for.
	CallSeq int64 `json:"call_seq"`
}

// Validate holds the payload to its shape.
func (p WorldAwaitedPayload) Validate() error {
	switch {
	case p.Tool == "":
		return fmt.Errorf("%w: world_awaited names no tool", ErrRehearsalStepInvalid)
	case p.CallSeq < FirstSeq:
		return fmt.Errorf("%w: world_awaited names no call", ErrRehearsalStepInvalid)
	}
	return nil
}

// WorldSuppliedPayload is the author's answer for the world, by reference.
type WorldSuppliedPayload struct {
	// AtSeq is the world_awaited step this answers.
	AtSeq int64 `json:"at_seq"`
	// Ref and Digest name the answer in the content store; Bytes is its size.
	Ref    string `json:"ref"`
	Digest string `json:"digest"`
	Bytes  int64  `json:"bytes,omitempty"`
	By     UserID `json:"by"`
}

// Validate holds the payload to its shape.
func (p WorldSuppliedPayload) Validate() error {
	switch {
	case p.AtSeq < FirstSeq:
		return fmt.Errorf("%w: world_supplied answers no request", ErrRehearsalStepInvalid)
	case p.Ref == "" || p.Digest == "":
		return fmt.Errorf("%w: world_supplied names no answer", ErrRehearsalStepInvalid)
	case p.By == "":
		return fmt.Errorf("%w: world_supplied names nobody", ErrRehearsalStepInvalid)
	}
	return nil
}

// RehearsalDecidedPayload is the author's decision on an action the Gate held in a
// rehearsal. It carries no note in this release.
type RehearsalDecidedPayload struct {
	// AtSeq is the approval_requested step this decides.
	AtSeq    int64  `json:"at_seq"`
	Approved bool   `json:"approved"`
	By       UserID `json:"by"`
}

// Validate holds the payload to its shape.
func (p RehearsalDecidedPayload) Validate() error {
	switch {
	case p.AtSeq < FirstSeq:
		return fmt.Errorf("%w: rehearsal_decided decides no request", ErrRehearsalStepInvalid)
	case p.By == "":
		return fmt.Errorf("%w: rehearsal_decided names nobody", ErrRehearsalStepInvalid)
	}
	return nil
}
