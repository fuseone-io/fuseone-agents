package connectortools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ticket"
)

var (
	ErrGraviteeAttemptClaim    = errors.New("connector: Gravitee attempt is not claimed by this worker")
	ErrGraviteeAttemptNotFound = errors.New("connector: Gravitee attempt was not found")
)

const graviteeAttemptKind = "gravitee.accept_subscription"

type GraviteeAttemptStatus string

const (
	GraviteeAttemptPrepared  GraviteeAttemptStatus = "prepared"
	GraviteeAttemptPending   GraviteeAttemptStatus = "pending"
	GraviteeAttemptConfirmed GraviteeAttemptStatus = "confirmed"
	GraviteeAttemptTerminal  GraviteeAttemptStatus = "terminal"
	GraviteeAttemptManual    GraviteeAttemptStatus = "manual"
)

type GraviteeAttempt struct {
	IdemKey        string
	Execution      ticket.Execution
	CallSeq        int64
	Instance       string
	Scope          domain.Scope
	ContractDigest string
	SubscriptionID string
	DecidedBy      domain.UserID
	Status         GraviteeAttemptStatus
	Result         ticket.ContentRef
	OutcomeCode    string
	Checks         int
	NextCheckAt    time.Time
	DeadlineAt     time.Time
	ClaimedBy      string
	ClaimedUntil   time.Time
	Settled        bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type GraviteeAttemptResolution struct {
	IdemKey     string
	ClaimedBy   string
	Status      GraviteeAttemptStatus
	Result      ticket.ContentRef
	OutcomeCode string
	NextCheckAt time.Time
	At          time.Time
}

type GraviteeAttemptJournal interface {
	Get(context.Context, string) (GraviteeAttempt, error)
	ClaimDue(context.Context, string, time.Time, time.Duration, int) ([]GraviteeAttempt, error)
	Arm(context.Context, string, string, time.Time, time.Time) (GraviteeAttempt, error)
	Resolve(context.Context, GraviteeAttemptResolution) (GraviteeAttempt, error)
	Settle(context.Context, string, time.Time) error
}

func validateGraviteeResolution(in GraviteeAttemptResolution) error {
	if strings.TrimSpace(in.IdemKey) == "" || in.At.IsZero() {
		return errors.New("connector: incomplete Gravitee attempt resolution")
	}
	switch in.Status {
	case GraviteeAttemptPrepared, GraviteeAttemptPending:
		if in.Result.Valid() || in.OutcomeCode != "" || in.NextCheckAt.IsZero() {
			return errors.New("connector: invalid pending Gravitee attempt")
		}
	case GraviteeAttemptConfirmed, GraviteeAttemptTerminal:
		if !in.Result.Valid() || strings.TrimSpace(in.OutcomeCode) == "" || !in.NextCheckAt.IsZero() {
			return errors.New("connector: incomplete final Gravitee attempt")
		}
	case GraviteeAttemptManual:
		if strings.TrimSpace(in.OutcomeCode) == "" || !in.NextCheckAt.IsZero() {
			return errors.New("connector: incomplete manual Gravitee attempt")
		}
	default:
		return fmt.Errorf("connector: unknown Gravitee attempt status %q", in.Status)
	}
	return nil
}

func graviteeAttemptFromExternal(in ticket.ExternalAttempt) (GraviteeAttempt, error) {
	if in.Kind != graviteeAttemptKind || !in.Execution.Valid() || in.CallSeq <= 0 ||
		!ValidInstanceName(in.Instance) || !in.Scope.Valid() ||
		strings.TrimSpace(in.ContractDigest) == "" || !graviteeID.MatchString(in.TargetID) ||
		in.DecidedBy == "" || in.NextCheckAt.IsZero() ||
		in.DeadlineAt.Before(in.NextCheckAt) || in.CreatedAt.IsZero() {
		return GraviteeAttempt{}, errors.New("connector: invalid durable Gravitee attempt")
	}
	return GraviteeAttempt{
		IdemKey: in.IdemKey, Execution: in.Execution, CallSeq: in.CallSeq,
		Instance: in.Instance, Scope: in.Scope, ContractDigest: in.ContractDigest,
		SubscriptionID: in.TargetID, DecidedBy: in.DecidedBy,
		Status: GraviteeAttemptPrepared, NextCheckAt: in.NextCheckAt,
		DeadlineAt: in.DeadlineAt, ClaimedBy: in.ClaimedBy,
		ClaimedUntil: in.ClaimedUntil, CreatedAt: in.CreatedAt, UpdatedAt: in.UpdatedAt,
	}, nil
}
