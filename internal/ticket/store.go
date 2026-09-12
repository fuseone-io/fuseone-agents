// Package ticket owns the durable state of governed support tickets.
package ticket

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

var (
	ErrNotFound         = errors.New("ticket: not found")
	ErrMoved            = errors.New("ticket: the revision moved")
	ErrNotRequester     = errors.New("ticket: only the requester may revise the ticket")
	ErrNotAddressSource = errors.New("ticket: only the configured source may address the ticket")
	ErrEventTaken       = errors.New("ticket: the event belongs to another ticket")
	ErrExecutionActive  = errors.New("ticket: another revision is executing")
	ErrAttemptConflict  = errors.New("ticket: the external attempt conflicts with its durable record")
	ErrSnapshotMoved    = errors.New("ticket: the inspected snapshot moved")
	ErrPhase            = errors.New("ticket: the revision is not in the required phase")
	ErrTerminal         = errors.New("ticket: the revision is terminal")
)

type Phase string

const (
	PhaseCollecting       Phase = "collecting"
	PhaseAwaitingApproval Phase = "awaiting_approval"
	PhaseExecuting        Phase = "executing"
	PhaseCompleted        Phase = "completed"
	PhaseRejected         Phase = "rejected"
	PhaseCancelled        Phase = "cancelled"
)

func (p Phase) terminal() bool {
	return p == PhaseCompleted || p == PhaseRejected || p == PhaseCancelled
}

type ContentRef struct {
	Ref    string
	Digest string
}

func (r ContentRef) Valid() bool {
	return strings.TrimSpace(r.Ref) != "" && strings.TrimSpace(r.Digest) != ""
}

type Approval struct {
	RunID    domain.RunID
	AtSeq    int64
	Snapshot ContentRef
}

type Execution struct {
	Ref           domain.TicketRef
	RunID         domain.RunID
	ApprovalAtSeq int64
	Snapshot      ContentRef
}

func (e Execution) Valid() bool {
	return e.Ref.Valid() && e.RunID != "" && e.ApprovalAtSeq > 0 && e.Snapshot.Valid()
}

type Outcome struct {
	Phase  Phase
	Result ContentRef
}

type Revision struct {
	Ref        domain.TicketRef
	Phase      Phase
	Draft      ContentRef
	Snapshot   ContentRef
	Approval   *Approval
	Recipients []domain.UserID
	Outcome    *Outcome
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Ticket struct {
	Key         domain.TicketKey
	Scope       domain.Scope
	RequestedBy domain.UserID
	AddressedBy string
	Current     Revision
	Active      *Execution
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type OpenInput struct {
	Key         domain.TicketKey
	Scope       domain.Scope
	RequestedBy domain.UserID
	AddressedBy string
	EventID     string
	Draft       ContentRef
	At          time.Time
}

type ReviseInput struct {
	Ref     domain.TicketRef
	EventID string
	By      domain.UserID
	Draft   ContentRef
	At      time.Time
}

type ApprovalInput struct {
	Ref      domain.TicketRef
	RunID    domain.RunID
	AtSeq    int64
	Snapshot ContentRef
	At       time.Time
}

type InspectionInput struct {
	Ref      domain.TicketRef
	Snapshot ContentRef
	At       time.Time
}

type AddressInput struct {
	Ref        domain.TicketRef
	EventID    string
	By         string
	Recipients []domain.UserID
	At         time.Time
}

type ClaimInput struct {
	Ref           domain.TicketRef
	RunID         domain.RunID
	ApprovalAtSeq int64
	At            time.Time
}

type FinishInput struct {
	Execution Execution
	Phase     Phase
	Result    ContentRef
	At        time.Time
}

type CloseInput struct {
	Ref    domain.TicketRef
	Phase  Phase
	Result ContentRef
	At     time.Time
}

type Store interface {
	Open(context.Context, OpenInput) (Ticket, bool, error)
	Current(context.Context, domain.TicketKey) (Ticket, error)
	Revision(context.Context, domain.TicketRef) (Revision, error)
	Revise(context.Context, ReviseInput) (Ticket, bool, error)
	Address(context.Context, AddressInput) (Ticket, bool, error)
	RecordInspection(context.Context, InspectionInput) (Ticket, bool, error)
	AwaitApproval(context.Context, ApprovalInput) (Ticket, bool, error)
	ClaimExecution(context.Context, ClaimInput) (Ticket, bool, error)
	ClaimExecutionWithAttempt(context.Context, ClaimAttemptInput) (Ticket, ExternalAttempt, bool, error)
	Close(context.Context, CloseInput) (Ticket, bool, error)
	FinishExecution(context.Context, FinishInput) (Ticket, error)
}

// Key names one Slack root without relying on a separator being absent from
// any vendor namespace.
func Key(connection, conversation, root string) (domain.TicketKey, error) {
	const maxPartBytes = 512
	parts := []string{connection, conversation, root}
	var out strings.Builder
	for _, part := range parts {
		if strings.TrimSpace(part) == "" || len(part) > maxPartBytes {
			return "", errors.New("ticket: every key part must be present and bounded")
		}
		out.WriteString(strconv.Itoa(len(part)))
		out.WriteByte(':')
		out.WriteString(part)
	}
	return domain.TicketKey(out.String()), nil
}

func validateOpen(in OpenInput) error {
	if strings.TrimSpace(string(in.Key)) == "" || !in.Scope.Valid() || in.RequestedBy == "" ||
		strings.TrimSpace(in.EventID) == "" || !in.Draft.Valid() || in.At.IsZero() {
		return errors.New("ticket: incomplete open request")
	}
	return nil
}

func validateRevise(in ReviseInput) error {
	if !in.Ref.Valid() || strings.TrimSpace(in.EventID) == "" || in.By == "" ||
		!in.Draft.Valid() || in.At.IsZero() {
		return errors.New("ticket: incomplete revision")
	}
	return nil
}

func validateApproval(in ApprovalInput) error {
	if !in.Ref.Valid() || in.RunID == "" || in.AtSeq <= 0 ||
		!in.Snapshot.Valid() || in.At.IsZero() {
		return errors.New("ticket: incomplete approval request")
	}
	return nil
}

func validateInspection(in InspectionInput) error {
	if !in.Ref.Valid() || !in.Snapshot.Valid() || in.At.IsZero() {
		return errors.New("ticket: incomplete inspection")
	}
	return nil
}

func validateAddress(in AddressInput) ([]domain.UserID, error) {
	if !in.Ref.Valid() || strings.TrimSpace(in.EventID) == "" ||
		strings.TrimSpace(in.By) == "" || in.At.IsZero() || len(in.Recipients) > 20 {
		return nil, errors.New("ticket: incomplete addressing request")
	}
	recipients := append([]domain.UserID(nil), in.Recipients...)
	for _, recipient := range recipients {
		if strings.TrimSpace(string(recipient)) == "" {
			return nil, errors.New("ticket: empty recipient")
		}
	}
	slices.Sort(recipients)
	recipients = slices.Compact(recipients)
	return recipients, nil
}

func validateClaim(in ClaimInput) error {
	if !in.Ref.Valid() || in.RunID == "" || in.ApprovalAtSeq <= 0 || in.At.IsZero() {
		return errors.New("ticket: incomplete execution claim")
	}
	return nil
}

func validateFinish(in FinishInput) error {
	if !in.Execution.Valid() || !in.Result.Valid() || in.At.IsZero() ||
		(in.Phase != PhaseCompleted && in.Phase != PhaseRejected) {
		return errors.New("ticket: incomplete execution result")
	}
	return nil
}

func validateClose(in CloseInput) error {
	if !in.Ref.Valid() || !in.Result.Valid() || in.At.IsZero() ||
		(in.Phase != PhaseRejected && in.Phase != PhaseCancelled) {
		return errors.New("ticket: incomplete terminal result")
	}
	return nil
}

func requireCurrent(ticket Ticket, ref domain.TicketRef) error {
	if ticket.Current.Ref != ref {
		return fmt.Errorf("%w: current revision is %d", ErrMoved, ticket.Current.Ref.Revision)
	}
	return nil
}

func requireMutable(ticket Ticket, ref domain.TicketRef) error {
	if err := requireCurrent(ticket, ref); err != nil {
		return err
	}
	if ticket.Current.Phase.terminal() {
		return ErrTerminal
	}
	return nil
}

func cloneTicket(in Ticket) Ticket {
	in.Current = cloneRevision(in.Current)
	if in.Active != nil {
		active := *in.Active
		in.Active = &active
	}
	return in
}

func cloneRevision(in Revision) Revision {
	in.Recipients = append([]domain.UserID(nil), in.Recipients...)
	if in.Approval != nil {
		approval := *in.Approval
		in.Approval = &approval
	}
	if in.Outcome != nil {
		outcome := *in.Outcome
		in.Outcome = &outcome
	}
	return in
}
