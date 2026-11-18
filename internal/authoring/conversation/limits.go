// Package conversation runs the workbench's turns (NT-018 phase 3): one
// message of an author, the model's answer, and the operations it proposes.
//
// Every bound here is deterministic, so a turn's worst case is known before
// the model is called: the message, the draft's projection and the window of
// conversation sent are capped, the output is capped, and so is the number of
// proposals one answer may carry. The full transcript is still stored; only
// what is sent is bounded.
package conversation

import (
	"errors"
	"time"
)

const (
	// MaxMessageBytes is the most one message may hold.
	MaxMessageBytes = 16 << 10
	// MaxProjectionBytes is the most the draft's projection may take: room
	// for a real agent's instructions and a scope's catalogue, whose input
	// still costs well under MaxTurnMicros at a frontier model's price.
	MaxProjectionBytes = 256 << 10
	// WindowTurns and WindowBytes bound the conversation sent with a turn.
	WindowTurns = 10
	WindowBytes = 32 << 10
	// MaxOutputTokens bounds the answer, and with it the reservation.
	MaxOutputTokens int64 = 4096
	// MaxProposalsPerTurn bounds what one answer may propose.
	MaxProposalsPerTurn = 8
	// MaxReasonBytes bounds a proposal's reason: one sentence beside a card.
	MaxReasonBytes = 500
	// MaxTurnMicros refuses a turn whose input alone costs more than this.
	MaxTurnMicros int64 = 2_000_000
	// Lease is how long a turn may stay in progress before a retry of its
	// key declares it unknown.
	Lease = 3 * time.Minute
)

var (
	ErrNoPrice         = errors.New("conversation: the model has no configured price, so a turn cannot be reserved")
	ErrTurnTooLarge    = errors.New("conversation: the turn is larger than a turn may be")
	ErrUnreadableReply = errors.New("conversation: the model's answer could not be read")
	ErrEmptyMessage    = errors.New("conversation: a turn needs a message")
)
