// Package ticket owns the durable state of governed support tickets.
package ticket

import (
	"context"
	"errors"
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
	ErrTooManyOpen      = errors.New("ticket: the scope has too many open tickets")
	ErrPhase            = errors.New("ticket: the revision is not in the required phase")
	ErrTerminal         = errors.New("ticket: the revision is terminal")
)

const MaxOpenPerScope = 200

type Phase string

const (
	PhaseCollecting       Phase = "collecting"
	PhaseAwaitingApproval Phase = "awaiting_approval"
	PhaseExecuting        Phase = "executing"
	PhaseNeedsAttention   Phase = "needs_attention"
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

// Origin is the vendor address of one support thread. It is stored beside the
// opaque key so a reply can find its ticket with an indexed equality lookup;
// decoding a length-prefixed key in SQL would turn every reply into a scan.
type Origin struct {
	Connection   string
	Conversation string
	Root         string
}

func (o Origin) Valid() bool {
	for _, part := range []string{o.Connection, o.Conversation, o.Root} {
		if strings.TrimSpace(part) == "" || len(part) > 512 {
			return false
		}
	}
	return true
}

func (r ContentRef) Valid() bool {
	return strings.TrimSpace(r.Ref) != "" && strings.TrimSpace(r.Digest) != ""
}

type Approval struct {
	RunID    domain.RunID
	AtSeq    int64
	Snapshot ContentRef
}

type SupersededApproval struct {
	Ref domain.TicketRef
	Approval
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
	Origin      Origin
	Scope       domain.Scope
	Agent       domain.AgentID
	RunAs       domain.UserID
	RequestedBy domain.UserID
	AddressedBy string
	Current     Revision
	Active      *Execution
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type OpenInput struct {
	Key         domain.TicketKey
	Origin      Origin
	Scope       domain.Scope
	Agent       domain.AgentID
	RunAs       domain.UserID
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

type AttentionInput struct {
	Execution Execution
	Result    ContentRef
	At        time.Time
}

type CloseInput struct {
	Ref    domain.TicketRef
	Phase  Phase
	Result ContentRef
	At     time.Time
}

// OutcomeNotice is one terminal external effect still owed to its support
// thread. Result is a reference to the safe projection, never to a credential.
type OutcomeNotice struct {
	Ref    domain.TicketRef
	Origin Origin
	Phase  Phase
	Result ContentRef
}

// ApprovalRoute is the immutable Slack origin and the people named for one
// ticket revision. It contains addressing, never authority: the reporter must
// still intersect Recipients with the people who may decide at send time.
type ApprovalRoute struct {
	Origin     Origin
	Recipients []domain.UserID
}

// ApprovalRoutes is the narrow ticket view used by approval delivery.
type ApprovalRoutes interface {
	ApprovalRoute(context.Context, domain.TicketRef) (ApprovalRoute, error)
}

// OutcomeNotices leases and settles terminal ticket notifications. It is
// separate from Store because a ticket writer has no reason to claim outbound
// work, and a notifier has no reason to revise a ticket.
type OutcomeNotices interface {
	ClaimOutcomes(context.Context, string, time.Time, time.Duration, int) ([]OutcomeNotice, error)
	MarkOutcomeAnnounced(context.Context, domain.TicketRef, string, time.Time) error
}

type Store interface {
	Open(context.Context, OpenInput) (Ticket, bool, error)
	Current(context.Context, domain.TicketKey) (Ticket, error)
	AtOrigin(context.Context, Origin) (Ticket, error)
	Revision(context.Context, domain.TicketRef) (Revision, error)
	SupersededApprovals(context.Context, domain.TicketRef) ([]SupersededApproval, error)
	MarkApprovalSuperseded(context.Context, domain.TicketRef, time.Time) error
	EventRevision(context.Context, string) (Ticket, Revision, error)
	Revise(context.Context, ReviseInput) (Ticket, bool, error)
	Address(context.Context, AddressInput) (Ticket, bool, error)
	RecordInspection(context.Context, InspectionInput) (Ticket, bool, error)
	AwaitApproval(context.Context, ApprovalInput) (Ticket, bool, error)
	ClaimExecution(context.Context, ClaimInput) (Ticket, bool, error)
	ClaimExecutionWithAttempt(context.Context, ClaimAttemptInput) (Ticket, ExternalAttempt, bool, error)
	MarkExecutionNeedsAttention(context.Context, AttentionInput) (Ticket, error)
	Close(context.Context, CloseInput) (Ticket, bool, error)
	FinishExecution(context.Context, FinishInput) (Ticket, error)
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
