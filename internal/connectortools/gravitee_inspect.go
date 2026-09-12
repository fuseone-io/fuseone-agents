package connectortools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ticket"
)

var (
	ErrGraviteeTicket    = errors.New("connector: the Gravitee call has no current ticket")
	ErrGraviteeRequester = errors.New("connector: the ticket requester does not own the application")
	ErrGraviteeTTL       = errors.New("connector: the requested expiration is outside policy")
	ErrGraviteeContract  = errors.New("connector: the Gravitee contract changed after approval")
	ErrGraviteeEvidence  = errors.New("connector: the approved Gravitee evidence is unavailable")
)

const ApprovalEvidenceGraviteeSubscription = "gravitee_subscription"

type GraviteeRemote interface {
	Inspect(context.Context, GraviteeConfig, SecretValue, string) (GraviteeObservation, error)
}

type RequesterEmails interface {
	EmailOf(context.Context, domain.UserID) (string, error)
}

type GraviteeTicketStore interface {
	Current(context.Context, domain.TicketKey) (ticket.Ticket, error)
	RecordInspection(context.Context, ticket.InspectionInput) (ticket.Ticket, bool, error)
}

type GraviteeInspectInput struct {
	SubscriptionID string `json:"subscriptionId"`
	ExpiresAt      string `json:"expiresAt,omitempty"`
}

type GraviteeSnapshot struct {
	TicketRef           domain.TicketRef    `json:"ticketRef"`
	SubscriptionID      string              `json:"subscriptionId"`
	Status              string              `json:"status"`
	Application         GraviteeApplication `json:"application"`
	API                 GraviteeResource    `json:"api"`
	Plan                GraviteeResource    `json:"plan"`
	PlanSecurity        string              `json:"planSecurity"`
	RequestedExpiration *string             `json:"requestedExpiration"`
	RemoteCreatedAt     string              `json:"remoteCreatedAt"`
	RemoteUpdatedAt     string              `json:"remoteUpdatedAt"`
}

type GraviteeInspector struct {
	access  GraviteeAccesses
	remote  GraviteeRemote
	people  RequesterEmails
	content engine.ContentStore
	tickets GraviteeTicketStore
	now     func() time.Time
}

func NewGraviteeInspector(
	access GraviteeAccesses, remote GraviteeRemote, people RequesterEmails,
	content engine.ContentStore, tickets GraviteeTicketStore,
) *GraviteeInspector {
	return &GraviteeInspector{
		access: access, remote: remote, people: people, content: content,
		tickets: tickets, now: time.Now,
	}
}

func (g *GraviteeInspector) Inspect(
	ctx context.Context, instance string, call engine.Call, input GraviteeInspectInput,
) (engine.ToolResult, error) {
	ticketState, err := g.currentTicket(ctx, call)
	if err != nil {
		return engine.ToolResult{}, err
	}
	access, err := g.access.Resolve(ctx, instance, call.Scope)
	if err != nil {
		return engine.ToolResult{}, err
	}
	expiresAt, err := requestedExpiration(input.ExpiresAt, access.Config, g.now().UTC())
	if err != nil {
		return engine.ToolResult{}, err
	}
	email, err := g.people.EmailOf(ctx, ticketState.RequestedBy)
	if err != nil {
		return engine.ToolResult{}, ErrGraviteeRequester
	}
	observed, err := g.remote.Inspect(ctx, access.Config, access.credential, input.SubscriptionID)
	if err != nil {
		return engine.ToolResult{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(email), observed.Application.PrimaryOwnerEmail) {
		return engine.ToolResult{}, ErrGraviteeRequester
	}
	snapshot := snapshotOf(call.Ticket.Ref, observed, expiresAt)
	return g.recordSnapshot(ctx, call, snapshot)
}

// ApprovalEvidence returns the exact safe snapshot already recorded for this
// ticket revision. It performs no remote read: asking Gravitee again here
// would let the card describe a different instant from the inspection result
// that led the model to propose the write.
func (g *GraviteeInspector) ApprovalEvidence(
	ctx context.Context, call engine.Call,
) (domain.ApprovalEvidence, error) {
	ticketState, err := g.currentTicket(ctx, call)
	if err != nil || !ticketState.Current.Snapshot.Valid() {
		return domain.ApprovalEvidence{}, ErrGraviteeTicket
	}
	raw, err := g.content.Get(ctx, ticketState.Current.Snapshot.Ref)
	evidence := domain.ApprovalEvidence{
		Kind: ApprovalEvidenceGraviteeSubscription, Ticket: call.Ticket.Ref,
		Ref: ticketState.Current.Snapshot.Ref, Digest: ticketState.Current.Snapshot.Digest,
	}
	if err != nil {
		return domain.ApprovalEvidence{}, ErrGraviteeTicket
	}
	if _, err := DecodeGraviteeApprovalEvidence(raw, evidence); err != nil {
		return domain.ApprovalEvidence{}, ErrGraviteeTicket
	}
	return evidence, nil
}

// DecodeGraviteeApprovalEvidence opens only the fixed snapshot shape and
// verifies the bytes against the ledger reference before any edge renders it.
func DecodeGraviteeApprovalEvidence(
	raw []byte, evidence domain.ApprovalEvidence,
) (GraviteeSnapshot, error) {
	if evidence.Kind != ApprovalEvidenceGraviteeSubscription || !evidence.Valid() ||
		engine.ResultDigest(raw) != evidence.Digest {
		return GraviteeSnapshot{}, ErrGraviteeTicket
	}
	var snapshot GraviteeSnapshot
	if json.Unmarshal(raw, &snapshot) != nil || !validGraviteeSnapshot(snapshot, evidence.Ticket) {
		return GraviteeSnapshot{}, ErrGraviteeTicket
	}
	return snapshot, nil
}

func validGraviteeSnapshot(snapshot GraviteeSnapshot, ref domain.TicketRef) bool {
	if snapshot.TicketRef != ref || !graviteeID.MatchString(snapshot.SubscriptionID) ||
		snapshot.Status != "PENDING" || snapshot.PlanSecurity != "API_KEY" ||
		!validRemoteResource(snapshot.Application.GraviteeResource) ||
		!validSnapshotOwner(snapshot.Application.PrimaryOwnerEmail) ||
		!validRemoteResource(snapshot.API) || !validRemoteResource(snapshot.Plan) ||
		!canonicalSnapshotTime(snapshot.RemoteCreatedAt) ||
		!canonicalSnapshotTime(snapshot.RemoteUpdatedAt) {
		return false
	}
	if snapshot.RequestedExpiration == nil {
		return true
	}
	parsed, err := time.Parse(time.RFC3339Nano, *snapshot.RequestedExpiration)
	return err == nil && parsed.UTC().Format(time.RFC3339Nano) == *snapshot.RequestedExpiration
}

func validSnapshotOwner(email string) bool {
	parsed, err := mail.ParseAddress(email)
	return err == nil && parsed.Address == email && strings.TrimSpace(email) == email && len(email) <= 320
}

func canonicalSnapshotTime(raw string) bool {
	canonical, ok := canonicalRemoteTime(raw)
	return ok && canonical == raw
}

func (g *GraviteeInspector) currentTicket(
	ctx context.Context, call engine.Call,
) (ticket.Ticket, error) {
	if g == nil || g.access == nil || g.remote == nil || g.people == nil ||
		g.content == nil || g.tickets == nil || !call.Ticket.Valid() {
		return ticket.Ticket{}, ErrGraviteeTicket
	}
	current, err := g.tickets.Current(ctx, call.Ticket.Ref.Key)
	if err != nil || current.Current.Ref != call.Ticket.Ref ||
		current.RequestedBy != call.Ticket.RequestedBy || current.Scope != call.Scope ||
		current.Current.Phase != ticket.PhaseCollecting {
		return ticket.Ticket{}, ErrGraviteeTicket
	}
	return current, nil
}

func requestedExpiration(raw string, cfg GraviteeConfig, now time.Time) (*string, error) {
	if strings.TrimSpace(raw) == "" {
		if cfg.AllowNoExpiry {
			return nil, nil
		}
		return nil, ErrGraviteeTTL
	}
	expires, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, ErrGraviteeTTL
	}
	ttl := expires.Sub(now)
	if ttl < time.Duration(cfg.MinTTLSeconds)*time.Second ||
		ttl > time.Duration(cfg.MaxTTLSeconds)*time.Second {
		return nil, ErrGraviteeTTL
	}
	canonical := expires.UTC().Format(time.RFC3339Nano)
	return &canonical, nil
}

func snapshotOf(
	ref domain.TicketRef, observed GraviteeObservation, expiresAt *string,
) GraviteeSnapshot {
	return GraviteeSnapshot{
		TicketRef: ref, SubscriptionID: observed.SubscriptionID, Status: observed.Status,
		Application: observed.Application, API: observed.API, Plan: observed.Plan,
		PlanSecurity: observed.PlanSecurity, RequestedExpiration: expiresAt,
		RemoteCreatedAt: observed.CreatedAt, RemoteUpdatedAt: observed.UpdatedAt,
	}
}

func (g *GraviteeInspector) recordSnapshot(
	ctx context.Context, call engine.Call, snapshot GraviteeSnapshot,
) (engine.ToolResult, error) {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return engine.ToolResult{}, fmt.Errorf("connector: encode Gravitee snapshot: %w", err)
	}
	ref, err := g.content.Put(ctx, call.RunID, call.Seq, raw)
	if err != nil {
		return engine.ToolResult{}, fmt.Errorf("connector: store Gravitee snapshot: %w", err)
	}
	digest := engine.ResultDigest(raw)
	_, _, err = g.tickets.RecordInspection(ctx, ticket.InspectionInput{
		Ref: call.Ticket.Ref, Snapshot: ticket.ContentRef{Ref: ref, Digest: digest}, At: g.now().UTC(),
	})
	if err != nil {
		return engine.ToolResult{}, err
	}
	return engine.ToolResult{
		ResultRef: ref, ResultDigest: digest, ResultBytes: int64(len(raw)),
		Labels: domain.NewLabels(domain.LabelUntrusted),
	}, nil
}
