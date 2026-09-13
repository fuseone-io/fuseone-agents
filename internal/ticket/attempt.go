package ticket

import (
	"errors"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// ExternalAttemptInput contains only the safe, durable coordinates needed to
// recover an external effect. Credentials and request bodies never belong in
// this record.
type ExternalAttemptInput struct {
	IdemKey        string
	Kind           string
	CallSeq        int64
	Instance       string
	Scope          domain.Scope
	ContractDigest string
	TargetID       string
	DecidedBy      domain.UserID
	NextCheckAt    time.Time
	DeadlineAt     time.Time
	ClaimedBy      string
	ClaimedUntil   time.Time
}

type ClaimAttemptInput struct {
	Claim   ClaimInput
	Attempt ExternalAttemptInput
}

type ExternalAttempt struct {
	ExternalAttemptInput
	Execution Execution
	CreatedAt time.Time
	UpdatedAt time.Time
}

func validateClaimAttempt(in ClaimAttemptInput) error {
	if err := validateClaim(in.Claim); err != nil {
		return err
	}
	a := in.Attempt
	if strings.TrimSpace(a.IdemKey) == "" || strings.TrimSpace(a.Kind) == "" ||
		a.CallSeq <= 0 || strings.TrimSpace(a.Instance) == "" || !a.Scope.Valid() ||
		strings.TrimSpace(a.ContractDigest) == "" || strings.TrimSpace(a.TargetID) == "" ||
		a.DecidedBy == "" || a.NextCheckAt.IsZero() ||
		a.DeadlineAt.Before(a.NextCheckAt) || strings.TrimSpace(a.ClaimedBy) == "" ||
		!a.ClaimedUntil.After(in.Claim.At) {
		return errors.New("ticket: incomplete external attempt")
	}
	return nil
}

func attemptFor(execution Execution, in ClaimAttemptInput) ExternalAttempt {
	return ExternalAttempt{
		ExternalAttemptInput: in.Attempt,
		Execution:            execution,
		CreatedAt:            in.Claim.At.UTC(),
		UpdatedAt:            in.Claim.At.UTC(),
	}
}

func sameExternalAttempt(a ExternalAttempt, b ExternalAttempt) bool {
	return a.IdemKey == b.IdemKey && a.Kind == b.Kind && a.CallSeq == b.CallSeq &&
		a.Instance == b.Instance && a.Scope == b.Scope &&
		a.ContractDigest == b.ContractDigest && a.TargetID == b.TargetID &&
		a.DecidedBy == b.DecidedBy && a.DeadlineAt.Equal(b.DeadlineAt) &&
		a.Execution == b.Execution
}
