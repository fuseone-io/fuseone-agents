// Package dedupe coordinates cross-run effect idempotency.
//
// It never decides what a tool may do. The Gate has already seen the proposal
// before a reservation is written, and only a successful ToolReturned confirms
// a row as done.
package dedupe

import (
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

type State string

const (
	StateReserved  State = "reserved"
	StatePending   State = "pending"
	StateConfirmed State = "confirmed"
)

var ErrReservationNotHeld = errors.New("dedupe: reservation is not held")

type Key struct {
	Scope       domain.Scope
	AgentID     domain.AgentID
	Tool        domain.ToolID
	Fingerprint string
}

func (k Key) Validate() error {
	if !k.Scope.Valid() {
		return fmt.Errorf("dedupe key needs a company and area scope")
	}
	if k.AgentID == "" {
		return fmt.Errorf("dedupe key needs an agent")
	}
	if k.Tool == "" {
		return fmt.Errorf("dedupe key needs a tool")
	}
	if k.Fingerprint == "" {
		return fmt.Errorf("dedupe key needs a fingerprint")
	}
	return nil
}

type Record struct {
	State     State
	RunID     domain.RunID
	Seq       int64
	ExpiresAt time.Time
}

func validateNow(now time.Time) error {
	if now.IsZero() {
		return fmt.Errorf("dedupe needs an observation time")
	}
	return nil
}
