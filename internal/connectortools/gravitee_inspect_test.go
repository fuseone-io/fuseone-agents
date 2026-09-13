package connectortools

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ticket"
)

var graviteeNow = time.Date(2026, 9, 11, 15, 0, 0, 0, time.UTC)

func TestGraviteeInspector_recordsTheOwnedSubscriptionForTheCurrentTicket(t *testing.T) {
	tickets, context := inspectedTicket(t)
	content := engine.NewMemoryContent()
	remote := &fakeGraviteeRemote{observation: safeObservation()}
	inspector := NewGraviteeInspector(
		&fakeGraviteeAccess{access: inspectionAccess()}, remote,
		fakeRequesterEmails{email: "dev@EXAMPLE.com"}, content, tickets)
	inspector.now = func() time.Time { return graviteeNow }
	call := inspectionCall(context)

	result, err := inspector.Inspect(t.Context(), "apim", call, GraviteeInspectInput{
		SubscriptionID: "sub-42", ExpiresAt: graviteeNow.Add(48 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	stored, err := content.Get(t.Context(), result.ResultRef)
	if err != nil {
		t.Fatalf("Get snapshot: %v", err)
	}
	var snapshot GraviteeSnapshot
	if err := json.Unmarshal(stored, &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if snapshot.TicketRef != context.Ref || snapshot.SubscriptionID != "sub-42" ||
		snapshot.Application.PrimaryOwnerEmail != "dev@example.com" ||
		snapshot.RequestedExpiration == nil {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	current, err := tickets.Current(t.Context(), context.Ref.Key)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if current.Current.Snapshot.Ref != result.ResultRef ||
		current.Current.Snapshot.Digest != result.ResultDigest {
		t.Fatalf("ticket snapshot = %+v, result = %+v", current.Current.Snapshot, result)
	}
	if !slices.Contains(result.Labels, domain.LabelUntrusted) || remote.calls != 1 {
		t.Fatalf("result labels = %v, remote calls = %d", result.Labels, remote.calls)
	}
}

func TestGraviteeInspector_doesNotFoldTheMailboxLocalPartForAuthorization(t *testing.T) {
	tickets, ticketContext := inspectedTicket(t)
	inspector := NewGraviteeInspector(
		&fakeGraviteeAccess{access: inspectionAccess()},
		&fakeGraviteeRemote{observation: safeObservation()},
		fakeRequesterEmails{email: "DEV@example.com"}, engine.NewMemoryContent(), tickets)
	inspector.now = func() time.Time { return graviteeNow }

	_, err := inspector.Inspect(t.Context(), "apim", inspectionCall(ticketContext),
		GraviteeInspectInput{
			SubscriptionID: "sub-42",
			ExpiresAt:      graviteeNow.Add(48 * time.Hour).Format(time.RFC3339),
		})
	if !errors.Is(err, ErrGraviteeRequester) {
		t.Fatalf("Inspect err = %v, want case-distinct local-part refused", err)
	}
}

func TestGraviteeInspector_returnsOnlyTheRecordedSnapshotAsApprovalEvidence(t *testing.T) {
	tickets, ticketContext := inspectedTicket(t)
	content := engine.NewMemoryContent()
	inspector := NewGraviteeInspector(
		&fakeGraviteeAccess{access: inspectionAccess()},
		&fakeGraviteeRemote{observation: safeObservation()},
		fakeRequesterEmails{email: "dev@example.com"}, content, tickets)
	inspector.now = func() time.Time { return graviteeNow }
	call := inspectionCall(ticketContext)
	if _, err := inspector.Inspect(t.Context(), "apim", call, GraviteeInspectInput{
		SubscriptionID: "sub-42", ExpiresAt: graviteeNow.Add(48 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	evidence, err := inspector.ApprovalEvidence(t.Context(), call)
	if err != nil {
		t.Fatalf("ApprovalEvidence: %v", err)
	}
	current, _ := tickets.Current(t.Context(), ticketContext.Ref.Key)
	if evidence.Kind != ApprovalEvidenceGraviteeSubscription ||
		evidence.Ticket != ticketContext.Ref ||
		evidence.Ref != current.Current.Snapshot.Ref ||
		evidence.Digest != current.Current.Snapshot.Digest {
		t.Fatalf("evidence = %+v, snapshot = %+v", evidence, current.Current.Snapshot)
	}
}

func TestGraviteeInspector_refusesEvidenceAfterTheTicketRevisionMoves(t *testing.T) {
	tickets, ticketContext := inspectedTicket(t)
	content := engine.NewMemoryContent()
	inspector := NewGraviteeInspector(
		&fakeGraviteeAccess{access: inspectionAccess()},
		&fakeGraviteeRemote{observation: safeObservation()},
		fakeRequesterEmails{email: "dev@example.com"}, content, tickets)
	inspector.now = func() time.Time { return graviteeNow }
	call := inspectionCall(ticketContext)
	if _, err := inspector.Inspect(t.Context(), "apim", call, GraviteeInspectInput{
		SubscriptionID: "sub-42", ExpiresAt: graviteeNow.Add(48 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if _, _, err := tickets.Revise(t.Context(), ticket.ReviseInput{
		Ref: ticketContext.Ref, EventID: "reply-after-inspection", By: ticketContext.RequestedBy,
		Draft: ticket.ContentRef{Ref: "draft://new", Digest: "digest-new"}, At: graviteeNow.Add(time.Minute),
	}); err != nil {
		t.Fatalf("Revise: %v", err)
	}

	if _, err := inspector.ApprovalEvidence(t.Context(), call); !errors.Is(err, ErrGraviteeTicket) {
		t.Fatalf("ApprovalEvidence err = %v, want stale ticket refusal", err)
	}
}

func TestGraviteeInspector_refusesTheWrongOwnerWithoutRecordingEvidence(t *testing.T) {
	tickets, context := inspectedTicket(t)
	observation := safeObservation()
	observation.Application.PrimaryOwnerEmail = "somebody-else@example.com"
	inspector := NewGraviteeInspector(
		&fakeGraviteeAccess{access: inspectionAccess()}, &fakeGraviteeRemote{observation: observation},
		fakeRequesterEmails{email: "dev@example.com"}, engine.NewMemoryContent(), tickets)
	inspector.now = func() time.Time { return graviteeNow }

	_, err := inspector.Inspect(t.Context(), "apim", inspectionCall(context), GraviteeInspectInput{
		SubscriptionID: "sub-42", ExpiresAt: graviteeNow.Add(48 * time.Hour).Format(time.RFC3339),
	})
	if !errors.Is(err, ErrGraviteeRequester) {
		t.Fatalf("Inspect err = %v, want ErrGraviteeRequester", err)
	}
	current, _ := tickets.Current(t.Context(), context.Ref.Key)
	if current.Current.Snapshot.Valid() {
		t.Fatalf("wrong owner recorded evidence: %+v", current.Current.Snapshot)
	}
}

func TestGraviteeInspector_refusesTTLBeforeContactingGravitee(t *testing.T) {
	tickets, context := inspectedTicket(t)
	remote := &fakeGraviteeRemote{observation: safeObservation()}
	inspector := NewGraviteeInspector(
		&fakeGraviteeAccess{access: inspectionAccess()}, remote,
		fakeRequesterEmails{email: "dev@example.com"}, engine.NewMemoryContent(), tickets)
	inspector.now = func() time.Time { return graviteeNow }

	_, err := inspector.Inspect(t.Context(), "apim", inspectionCall(context), GraviteeInspectInput{
		SubscriptionID: "sub-42", ExpiresAt: graviteeNow.Add(time.Hour).Format(time.RFC3339),
	})
	if !errors.Is(err, ErrGraviteeTTL) || remote.calls != 0 {
		t.Fatalf("Inspect = (%v), remote calls = %d", err, remote.calls)
	}
}

func TestGraviteeInspector_aStaleTicketReachesNoAuthority(t *testing.T) {
	tickets, context := inspectedTicket(t)
	access := &fakeGraviteeAccess{access: inspectionAccess()}
	_, _, err := tickets.Revise(t.Context(), ticket.ReviseInput{
		Ref: context.Ref, EventID: "reply-2", By: context.RequestedBy,
		Draft: ticket.ContentRef{Ref: "draft://2", Digest: "digest-2"}, At: graviteeNow,
	})
	if err != nil {
		t.Fatalf("Revise: %v", err)
	}
	inspector := NewGraviteeInspector(access, &fakeGraviteeRemote{observation: safeObservation()},
		fakeRequesterEmails{email: "dev@example.com"}, engine.NewMemoryContent(), tickets)

	_, err = inspector.Inspect(t.Context(), "apim", inspectionCall(context), GraviteeInspectInput{
		SubscriptionID: "sub-42", ExpiresAt: graviteeNow.Add(48 * time.Hour).Format(time.RFC3339),
	})
	if !errors.Is(err, ErrGraviteeTicket) || access.calls != 0 {
		t.Fatalf("Inspect = %v, authority calls = %d", err, access.calls)
	}
}

func inspectedTicket(t *testing.T) (*ticket.Memory, domain.TicketContext) {
	t.Helper()
	store := ticket.NewMemory()
	key, err := ticket.Key("slack", "C-support", "1700.1")
	if err != nil {
		t.Fatalf("Key: %v", err)
	}
	opened, _, err := store.Open(t.Context(), ticket.OpenInput{
		Key: key, Origin: ticket.Origin{
			Connection: "slack", Conversation: "C-support", Root: "1700.1",
		}, Scope: area("acme", "platform"), Agent: "gateway-support", RunAs: "usr_gateway",
		RequestedBy: "usr_dev",
		AddressedBy: "slack-bot", EventID: "event-1",
		Draft: ticket.ContentRef{Ref: "draft://1", Digest: "digest-1"}, At: graviteeNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return store, domain.TicketContext{
		Ref: opened.Current.Ref, RequestedBy: opened.RequestedBy, AddressedBy: opened.AddressedBy,
	}
}

func inspectionCall(context domain.TicketContext) engine.Call {
	return engine.Call{
		RunID: "run-1", Seq: 4, Scope: area("acme", "platform"), Ticket: context,
	}
}

func inspectionAccess() GraviteeAccess {
	cfg := graviteeInstance(area("acme", "platform"), graviteeSource("secrets")).Gravitee
	return GraviteeAccess{Config: cfg, credential: SecretValue{value: "token"}}
}

func safeObservation() GraviteeObservation {
	return GraviteeObservation{
		SubscriptionID: "sub-42", Status: "PENDING",
		Application: GraviteeApplication{
			GraviteeResource:  GraviteeResource{ID: "app", Name: "Portal"},
			PrimaryOwnerEmail: "dev@example.com",
		},
		API:  GraviteeResource{ID: "checkout-api", Name: "Checkout"},
		Plan: GraviteeResource{ID: "plan", Name: "Keys"}, PlanSecurity: "API_KEY",
		CreatedAt: "2026-09-10T12:00:00Z", UpdatedAt: "2026-09-11T12:00:00Z",
	}
}

type fakeGraviteeAccess struct {
	access GraviteeAccess
	err    error
	calls  int
}

func (f *fakeGraviteeAccess) Resolve(context.Context, string, domain.Scope) (GraviteeAccess, error) {
	f.calls++
	return f.access, f.err
}

type fakeGraviteeRemote struct {
	observation GraviteeObservation
	err         error
	calls       int
}

func (f *fakeGraviteeRemote) Inspect(
	context.Context, GraviteeConfig, SecretValue, string,
) (GraviteeObservation, error) {
	f.calls++
	return f.observation, f.err
}

type fakeRequesterEmails struct {
	email string
	err   error
}

func (f fakeRequesterEmails) EmailOf(context.Context, domain.UserID) (string, error) {
	return f.email, f.err
}
