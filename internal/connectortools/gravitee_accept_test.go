package connectortools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ticket"
)

func TestGraviteeAcceptRuntime_concurrentRetriesEmitOnePOST(t *testing.T) {
	fixture := newAcceptanceFixture(t)
	results := make(chan engine.ToolResult, 8)
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := fixture.runtime.Accept(t.Context(), "apim", fixture.call, fixture.input)
			results <- result
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Accept: %v", err)
		}
	}
	for range results {
	}
	if fixture.remote.acceptCalls != 1 {
		t.Fatalf("POST calls = %d, want exactly one", fixture.remote.acceptCalls)
	}
	current, err := fixture.tickets.Current(t.Context(), fixture.call.Ticket.Ref.Key)
	if err != nil || current.Current.Phase != ticket.PhaseCompleted || current.Active != nil {
		t.Fatalf("completed ticket = (%+v, %v)", current, err)
	}
	stored, err := fixture.attempts.Get(t.Context(), fixture.call.IdemKey)
	if err != nil || !stored.Settled || stored.Status != GraviteeAttemptConfirmed {
		t.Fatalf("settled attempt = (%+v, %v)", stored, err)
	}
	raw, err := fixture.content.Get(t.Context(), stored.Result.Ref)
	if err != nil {
		t.Fatalf("result content: %v", err)
	}
	if strings.Contains(string(raw), "token") || strings.Contains(string(raw), "apiKey") {
		t.Fatalf("safe result contains authority: %s", raw)
	}
}

func TestGraviteeAcceptRuntime_aPreparedAttemptRecoversTheFirstPOST(t *testing.T) {
	fixture := newAcceptanceFixture(t)
	fixture.access.err = errors.New("vault unavailable")
	result, err := fixture.runtime.Accept(t.Context(), "apim", fixture.call, fixture.input)
	if err != nil || !result.Failed || result.ErrorCode != CodeConnectorOutcomeUnknown {
		t.Fatalf("initial Accept = (%+v, %v)", result, err)
	}
	if fixture.remote.acceptCalls != 0 {
		t.Fatalf("POST happened without authority: %d", fixture.remote.acceptCalls)
	}
	fixture.access.err = nil
	fixture.advance(6 * time.Second)
	reconciler := NewGraviteeAttemptReconciler(fixture.runtime, fixture.attempts, "worker-a")
	reconciler.now = fixture.runtime.now
	if count, err := reconciler.Sweep(t.Context()); err != nil || count != 1 {
		t.Fatalf("Sweep = (%d, %v)", count, err)
	}
	if fixture.remote.acceptCalls != 1 || fixture.remote.inspectCalls != 1 {
		t.Fatalf("recovered calls inspect=%d POST=%d",
			fixture.remote.inspectCalls, fixture.remote.acceptCalls)
	}
}

func TestGraviteeAcceptRuntime_anAmbiguousPOSTIsResolvedOnlyByGET(t *testing.T) {
	fixture := newAcceptanceFixture(t)
	fixture.remote.acceptErr = graviteeRemoteError{status: 503, ambiguous: true}
	fixture.remote.observed = acceptedObservation(fixture.snapshot)
	result, err := fixture.runtime.Accept(t.Context(), "apim", fixture.call, fixture.input)
	if err != nil || !result.Failed || result.ErrorCode != CodeConnectorOutcomeUnknown {
		t.Fatalf("ambiguous Accept = (%+v, %v)", result, err)
	}
	fixture.advance(graviteeFirstCheck + time.Second)
	reconciler := NewGraviteeAttemptReconciler(fixture.runtime, fixture.attempts, "worker-b")
	reconciler.now = fixture.runtime.now
	if count, err := reconciler.Sweep(t.Context()); err != nil || count != 1 {
		t.Fatalf("Sweep = (%d, %v)", count, err)
	}
	if fixture.remote.acceptCalls != 1 || fixture.remote.observeCalls != 1 {
		t.Fatalf("ambiguous recovery made inspect=%d POST=%d GET=%d",
			fixture.remote.inspectCalls, fixture.remote.acceptCalls, fixture.remote.observeCalls)
	}
	attempt, _ := fixture.attempts.Get(t.Context(), fixture.call.IdemKey)
	raw, err := fixture.content.Get(t.Context(), attempt.Result.Ref)
	if err != nil {
		t.Fatalf("recovered result: %v", err)
	}
	var decoded GraviteeAcceptanceResult
	if json.Unmarshal(raw, &decoded) != nil || !decoded.Recovered || decoded.Status != "accepted" {
		t.Fatalf("recovered result = %s", raw)
	}
}

func TestGraviteeAcceptRuntime_uncertaintyPastTheDeadlineNeedsAttention(t *testing.T) {
	fixture := newAcceptanceFixture(t)
	fixture.access.err = errors.New("vault unavailable")
	if _, err := fixture.runtime.Accept(t.Context(), "apim", fixture.call, fixture.input); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	fixture.advance(graviteeDeadline + time.Second)
	reconciler := NewGraviteeAttemptReconciler(fixture.runtime, fixture.attempts, "worker-c")
	reconciler.now = fixture.runtime.now
	if count, err := reconciler.Sweep(t.Context()); err != nil || count != 1 {
		t.Fatalf("Sweep = (%d, %v)", count, err)
	}
	attempt, err := fixture.attempts.Get(t.Context(), fixture.call.IdemKey)
	if err != nil || attempt.Status != GraviteeAttemptManual || attempt.Settled {
		t.Fatalf("manual attempt = (%+v, %v)", attempt, err)
	}
	current, _ := fixture.tickets.Current(t.Context(), fixture.call.Ticket.Ref.Key)
	if current.Current.Phase != ticket.PhaseExecuting || fixture.remote.acceptCalls != 0 {
		t.Fatalf("manual ticket = %+v, POST calls = %d", current, fixture.remote.acceptCalls)
	}
}

type acceptanceFixture struct {
	runtime  *GraviteeAcceptRuntime
	tickets  ticket.Store
	attempts *memoryGraviteeAttempts
	content  *engine.MemoryContent
	access   *fakeGraviteeAccess
	remote   *fakeAcceptanceRemote
	call     engine.Call
	input    GraviteeInspectInput
	snapshot GraviteeSnapshot
	now      time.Time
}

func newAcceptanceFixture(t *testing.T) *acceptanceFixture {
	t.Helper()
	base, ticketContext := inspectedTicket(t)
	content := engine.NewMemoryContent()
	expiration := graviteeNow.Add(48 * time.Hour).Format(time.RFC3339Nano)
	snapshot := snapshotOf(ticketContext.Ref, safeObservation(), &expiration)
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("encode snapshot: %v", err)
	}
	ref, err := content.Put(t.Context(), "run-inspect", 4, raw)
	if err != nil {
		t.Fatalf("store snapshot: %v", err)
	}
	evidence := domain.ApprovalEvidence{
		Kind: ApprovalEvidenceGraviteeSubscription, Ticket: ticketContext.Ref,
		Ref: ref, Digest: engine.ResultDigest(raw),
	}
	if _, _, err := base.RecordInspection(t.Context(), ticket.InspectionInput{
		Ref: ticketContext.Ref, Snapshot: ticket.ContentRef{Ref: ref, Digest: evidence.Digest},
		At: graviteeNow,
	}); err != nil {
		t.Fatalf("record snapshot: %v", err)
	}
	journal := newMemoryGraviteeAttempts()
	store := &journalTicketStore{Store: base, attempts: journal}
	access := &fakeGraviteeAccess{access: inspectionAccess()}
	access.access.ContractDigest = "gravitee-contract/v1:sha256:fixed"
	remote := &fakeAcceptanceRemote{
		inspected: safeObservation(), observed: safeObservation(),
		accepted: acceptedObservation(snapshot),
	}
	fixture := &acceptanceFixture{
		tickets: store, attempts: journal, content: content, access: access, remote: remote,
		call: engine.Call{
			RunID: "run-accept", Seq: 9, Scope: area("acme", "platform"),
			Ticket: ticketContext, ContractDigest: access.access.ContractDigest,
			ApprovalEvidence: evidence, ApprovalAtSeq: 7, DecidedBy: "usr_approver",
			IdemKey: "gravitee-accept-once", At: graviteeNow,
		},
		input:    GraviteeInspectInput{SubscriptionID: "sub-42", ExpiresAt: expiration},
		snapshot: snapshot, now: graviteeNow,
	}
	fixture.runtime = NewGraviteeAcceptRuntime(access, remote, content, store, journal)
	fixture.runtime.now = func() time.Time { return fixture.now }
	return fixture
}

func (f *acceptanceFixture) advance(elapsed time.Duration) { f.now = f.now.Add(elapsed) }

func acceptedObservation(snapshot GraviteeSnapshot) GraviteeObservation {
	return GraviteeObservation{
		SubscriptionID: snapshot.SubscriptionID, Status: "ACCEPTED",
		Application: snapshot.Application, API: snapshot.API, Plan: snapshot.Plan,
		PlanSecurity: snapshot.PlanSecurity, CreatedAt: snapshot.RemoteCreatedAt,
		UpdatedAt: "2026-09-11T15:01:00Z", EndingAt: snapshot.RequestedExpiration,
	}
}

type fakeAcceptanceRemote struct {
	mu           sync.Mutex
	inspected    GraviteeObservation
	observed     GraviteeObservation
	accepted     GraviteeObservation
	inspectErr   error
	observeErr   error
	acceptErr    error
	inspectCalls int
	observeCalls int
	acceptCalls  int
}

func (f *fakeAcceptanceRemote) Inspect(
	context.Context, GraviteeConfig, SecretValue, string,
) (GraviteeObservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inspectCalls++
	return f.inspected, f.inspectErr
}

func (f *fakeAcceptanceRemote) Observe(
	context.Context, GraviteeConfig, SecretValue, string,
) (GraviteeObservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.observeCalls++
	return f.observed, f.observeErr
}

func (f *fakeAcceptanceRemote) Accept(
	context.Context, GraviteeConfig, SecretValue, GraviteeSnapshot,
) (GraviteeObservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acceptCalls++
	return f.accepted, f.acceptErr
}

type journalTicketStore struct {
	ticket.Store
	attempts *memoryGraviteeAttempts
}

func (s *journalTicketStore) ClaimExecutionWithAttempt(
	ctx context.Context, in ticket.ClaimAttemptInput,
) (ticket.Ticket, ticket.ExternalAttempt, bool, error) {
	got, attempt, created, err := s.Store.ClaimExecutionWithAttempt(ctx, in)
	if err == nil && created {
		if err := s.attempts.seed(attempt); err != nil {
			return ticket.Ticket{}, ticket.ExternalAttempt{}, false, err
		}
	}
	return got, attempt, created, err
}
