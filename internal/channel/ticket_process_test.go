package channel_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/ticket"
)

var ticketNow = time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
var channelTicketKey = func() domain.TicketKey {
	key, err := ticket.Key("acme-slack", "C-tickets", "171.1")
	if err != nil {
		panic(err)
	}
	return key
}()

func TestTicketHandler_aLinkedRootOpensOneGovernedRevision(t *testing.T) {
	handler, store, opener, _ := ticketHandler()
	result, err := handler.Handle(t.Context(), ticketRoot("event-root", "Create an API key for checkout"))
	if err != nil || result.RunID == "" {
		t.Fatalf("Handle: result=%+v err=%v", result, err)
	}
	held, err := store.Current(t.Context(), channelTicketKey)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if held.RequestedBy != "usr_requester" || held.RunAs != "usr_gateway" ||
		held.Agent != "gateway-support" || held.Current.Ref.Revision != 1 {
		t.Fatalf("ticket = %+v", held)
	}
	request := opener.last()
	if request.By != "usr_gateway" || request.Ticket == nil ||
		request.Ticket.RequestedBy != "usr_requester" ||
		request.Origin == nil || request.Origin.Thread != "171.1" ||
		!request.Labels.Has(domain.LabelUntrusted) {
		t.Fatalf("request = %+v, want configured authority and untrusted evidence", request)
	}
	var input struct {
		Messages []struct {
			By   string `json:"by"`
			Text string `json:"text"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(request.Input, &input); err != nil {
		t.Fatalf("input: %v", err)
	}
	if len(input.Messages) != 1 || input.Messages[0].By != "usr_requester" ||
		input.Messages[0].Text != "Create an API key for checkout" {
		t.Fatalf("input = %+v", input)
	}
}

func TestTicketHandler_onlyTheRequesterRevisesTheTicket(t *testing.T) {
	handler, store, opener, people := ticketHandler()
	mustHandleTicket(t, handler, ticketRoot("event-root", "Create an API key"))
	people.byAccount["U-stranger"] = "usr_stranger"

	stranger := ticketReply("event-stranger", "171.2", "I changed the TTL")
	stranger.Source = channel.Source{User: "U-stranger"}
	result := mustHandleTicket(t, handler, stranger)
	if result.HandledReason != "ticket_author_ignored" || opener.count() != 1 {
		t.Fatalf("stranger result=%+v, runs=%d", result, opener.count())
	}

	result = mustHandleTicket(t, handler,
		ticketReply("event-requester", "171.3", "Expire it in 30 days"))
	if result.RunID == "" || opener.count() != 2 {
		t.Fatalf("requester result=%+v, runs=%d", result, opener.count())
	}
	held, _ := store.Current(t.Context(), channelTicketKey)
	if held.Current.Ref.Revision != 2 {
		t.Fatalf("revision = %d, want 2", held.Current.Ref.Revision)
	}
}

func TestTicketHandler_theConfiguredActorMayAddressOnlyRealDeciders(t *testing.T) {
	handler, store, opener, people := ticketHandler()
	mustHandleTicket(t, handler, ticketRoot("event-root", "Create an API key"))
	people.byAccount["UMANAGER"] = "usr_manager"
	people.byAccount["UVIEWER"] = "usr_viewer"

	reply := ticketReply("event-address", "171.2", "Approval: <@UMANAGER> <@UVIEWER>")
	reply.Source = channel.Source{Bot: "B-approvals", App: "A-also-present"}
	result := mustHandleTicket(t, handler, reply)
	if result.RunID == "" || opener.count() != 2 {
		t.Fatalf("address result=%+v runs=%d", result, opener.count())
	}
	held, _ := store.Current(t.Context(), channelTicketKey)
	if len(held.Current.Recipients) != 1 || held.Current.Recipients[0] != "usr_manager" {
		t.Fatalf("recipients = %v, want only the authorised manager", held.Current.Recipients)
	}
}

func TestTicketHandler_twoRequesterRepliesLoseNeitherMessage(t *testing.T) {
	handler, store, opener, _ := ticketHandler()
	mustHandleTicket(t, handler, ticketRoot("event-root", "Create an API key"))

	start := make(chan struct{})
	results := make(chan error, 2)
	for _, reply := range []channel.Claimed{
		ticketReply("event-a", "171.2", "Application checkout"),
		ticketReply("event-b", "171.3", "Expire in 30 days"),
	} {
		go func(reply channel.Claimed) {
			<-start
			_, err := handler.Handle(context.Background(), reply)
			results <- err
		}(reply)
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("Handle: %v", err)
		}
	}
	held, _ := store.Current(t.Context(), channelTicketKey)
	if held.Current.Ref.Revision != 3 {
		t.Fatalf("revision = %d, want both replies at revision 3", held.Current.Ref.Revision)
	}
	request, ok := opener.revision(held.Current.Ref)
	if !ok {
		t.Fatalf("no run opened for current revision %+v", held.Current.Ref)
	}
	for _, want := range []string{
		"Create an API key", "Application checkout", "Expire in 30 days",
	} {
		if !strings.Contains(string(request.Input), want) {
			t.Fatalf("current draft = %s, missing %q", request.Input, want)
		}
	}
}

func TestTicketHandler_replayingAnOlderEventDoesNotResurrectItsRun(t *testing.T) {
	handler, _, opener, _ := ticketHandler()
	mustHandleTicket(t, handler, ticketRoot("event-root", "Create an API key"))
	old := ticketReply("event-old", "171.2", "Expire in 30 days")
	mustHandleTicket(t, handler, old)
	mustHandleTicket(t, handler, ticketReply("event-new", "171.3", "Make that 15 days"))

	before := opener.count()
	result := mustHandleTicket(t, handler, old)
	if result.HandledReason != "ticket_revision_already_advanced" || opener.count() != before {
		t.Fatalf("replay result=%+v runs=%d before=%d", result, opener.count(), before)
	}
}

func TestTicketHandler_replayingARevisionClosesItsSupersededApprovalBeforeOpening(t *testing.T) {
	handler, store, opener, _, runs := ticketHandlerWithRuns()
	root := mustHandleTicket(t, handler,
		ticketRoot("event-root", "Create an API key"))
	opened, err := store.Current(t.Context(), channelTicketKey)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	awaitTicketApproval(t, runs, store, opened, root.RunID)

	// The ticket transaction committed, then the process died before it could
	// close the old approval or open the new run. Replaying the persisted
	// event must finish both obligations.
	revised, changed, err := store.Revise(t.Context(), ticket.ReviseInput{
		Ref: opened.Current.Ref, EventID: "event-revision",
		By: opened.RequestedBy, Draft: opened.Current.Draft,
		At: ticketNow.Add(time.Minute),
	})
	if err != nil || !changed {
		t.Fatalf("Revise = (%+v, %v, %v)", revised, changed, err)
	}
	result := mustHandleTicket(t, handler,
		ticketReply("event-revision", "171.2", "Expire it in 15 days"))
	if result.RunID == "" || opener.count() != 2 {
		t.Fatalf("replay = %+v, opened %d runs", result, opener.count())
	}

	steps, err := runs.Read(t.Context(), root.RunID, domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read superseded run: %v", err)
	}
	last := steps[len(steps)-1]
	if last.Kind != domain.StepFailed {
		t.Fatalf("superseded run ends at %s, want failed", last.Kind)
	}
	var failure domain.FailedPayload
	if err := json.Unmarshal(last.Payload, &failure); err != nil {
		t.Fatalf("decode failure: %v", err)
	}
	if failure.Code != "ticket_revision_superseded" || failure.Retryable {
		t.Fatalf("failure = %+v", failure)
	}
	pending, err := store.SupersededApprovals(t.Context(), revised.Current.Ref)
	if err != nil || len(pending) != 0 {
		t.Fatalf("superseded obligation survived = (%+v, %v)", pending, err)
	}
}

func TestTicketHandler_aRevisionWaitsUntilTheClaimedExecutionFinishes(t *testing.T) {
	handler, store, opener, _, runs := ticketHandlerWithRuns()
	root := mustHandleTicket(t, handler,
		ticketRoot("event-root", "Create an API key"))
	opened, err := store.Current(t.Context(), channelTicketKey)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	awaitTicketApproval(t, runs, store, opened, root.RunID)
	claimed, changed, err := store.ClaimExecution(t.Context(), ticket.ClaimInput{
		Ref: opened.Current.Ref, RunID: root.RunID, ApprovalAtSeq: 2,
		At: ticketNow.Add(time.Minute),
	})
	if err != nil || !changed || claimed.Active == nil {
		t.Fatalf("ClaimExecution = (%+v, %v, %v)", claimed, changed, err)
	}
	revised, changed, err := store.Revise(t.Context(), ticket.ReviseInput{
		Ref: claimed.Current.Ref, EventID: "event-revision",
		By: claimed.RequestedBy, Draft: claimed.Current.Draft,
		At: ticketNow.Add(2 * time.Minute),
	})
	if err != nil || !changed {
		t.Fatalf("Revise = (%+v, %v, %v)", revised, changed, err)
	}

	_, err = handler.Handle(t.Context(),
		ticketReply("event-revision", "171.2", "Expire it in 15 days"))
	if !errors.Is(err, ticket.ErrExecutionActive) || opener.count() != 1 {
		t.Fatalf("while executing = (runs %d, %v), want a durable retry", opener.count(), err)
	}
	if _, err := store.FinishExecution(t.Context(), ticket.FinishInput{
		Execution: *claimed.Active, Phase: ticket.PhaseCompleted,
		Result: ticket.ContentRef{Ref: "content://done", Digest: "sha256:done"},
		At:     ticketNow.Add(3 * time.Minute),
	}); err != nil {
		t.Fatalf("FinishExecution: %v", err)
	}
	result := mustHandleTicket(t, handler,
		ticketReply("event-revision", "171.2", "Expire it in 15 days"))
	if result.RunID == "" || opener.count() != 2 {
		t.Fatalf("after finish = %+v, opened %d runs", result, opener.count())
	}
}

func TestTicketHandler_aReplyAfterTheTicketClosedIsSettledWithoutAnotherRun(t *testing.T) {
	handler, store, opener, _ := ticketHandler()
	mustHandleTicket(t, handler, ticketRoot("event-root", "Create an API key"))
	held, err := store.Current(t.Context(), channelTicketKey)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	_, _, err = store.Close(t.Context(), ticket.CloseInput{
		Ref: held.Current.Ref, Phase: ticket.PhaseRejected,
		Result: ticket.ContentRef{Ref: "content://rejected", Digest: "sha256:rejected"},
		At:     ticketNow.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("Close: %v", err)
	}

	result := mustHandleTicket(t, handler,
		ticketReply("event-after-close", "171.4", "Try another expiration"))
	if result.HandledReason != "ticket_already_closed" || opener.count() != 1 {
		t.Fatalf("closed reply = %+v, runs=%d", result, opener.count())
	}
}

type ticketPeople struct {
	byAccount map[string]domain.UserID
}

func (p *ticketPeople) PrincipalFor(
	_ context.Context, _ string, account string,
) (domain.UserID, bool, error) {
	who, ok := p.byAccount[account]
	return who, ok, nil
}

func (p *ticketPeople) PrincipalsOn(
	_ context.Context, _ string, accounts []string,
) (map[string]domain.UserID, error) {
	out := make(map[string]domain.UserID, len(accounts))
	for _, account := range accounts {
		if who := p.byAccount[account]; who != "" {
			out[account] = who
		}
	}
	return out, nil
}

type ticketDeciders []domain.UserID

func (d ticketDeciders) DecidersIn(context.Context, domain.Scope) ([]domain.UserID, error) {
	return append([]domain.UserID(nil), d...), nil
}

type ticketOpener struct {
	mu       sync.Mutex
	requests []channel.Request
}

func (o *ticketOpener) Open(_ context.Context, request channel.Request) (channel.Opened, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.requests = append(o.requests, request)
	return channel.Opened{RunID: domain.RunID("run-ticket-" + request.IdemKey), Created: true}, nil
}

func (o *ticketOpener) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.requests)
}

func (o *ticketOpener) last() channel.Request {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.requests[len(o.requests)-1]
}

func (o *ticketOpener) revision(ref domain.TicketRef) (channel.Request, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, request := range o.requests {
		if request.Ticket != nil && request.Ticket.Ref == ref {
			return request, true
		}
	}
	return channel.Request{}, false
}

func ticketHandler() (*channel.TicketHandler, *ticket.Memory, *ticketOpener, *ticketPeople) {
	handler, store, opener, people, _ := ticketHandlerWithRuns()
	return handler, store, opener, people
}

func ticketHandlerWithRuns() (
	*channel.TicketHandler, *ticket.Memory, *ticketOpener, *ticketPeople, *ledger.Memory,
) {
	store := ticket.NewMemory()
	content := engine.NewMemoryContent()
	opener := &ticketOpener{}
	runs := ledger.NewMemory()
	people := &ticketPeople{byAccount: map[string]domain.UserID{
		"U-requester": "usr_requester",
	}}
	handler := channel.NewTicketHandler(
		store, content, opener, people, people, ticketDeciders{"usr_manager"},
		runs, func() time.Time { return ticketNow },
	)
	return handler, store, opener, people, runs
}

func ticketRoot(event, text string) channel.Claimed {
	return channel.Claimed{Arrival: channel.Arrival{
		Channel: "acme-slack", Conversation: "C-tickets", EventID: event,
		Message: "171.1", Thread: "171.1", Text: text,
		Source: channel.Source{User: "U-requester"},
		Ticket: &channel.TicketIntent{
			Key: channelTicketKey, Root: true,
			Scope: domain.Scope{Company: "acme", Area: "support"},
			Agent: "gateway-support", RunAs: "usr_gateway", AddressedBy: "bot:B-approvals",
		},
	}}
}

func ticketReply(event, message, text string) channel.Claimed {
	return channel.Claimed{Arrival: channel.Arrival{
		Channel: "acme-slack", Conversation: "C-tickets", EventID: event,
		Message: message, Thread: "171.1", Text: text,
		Source: channel.Source{User: "U-requester"},
		Ticket: &channel.TicketIntent{Key: channelTicketKey},
	}}
}

func mustHandleTicket(
	t *testing.T, handler *channel.TicketHandler, arrival channel.Claimed,
) channel.TicketResult {
	t.Helper()
	result, err := handler.Handle(t.Context(), arrival)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	return result
}

func awaitTicketApproval(
	t *testing.T, runs *ledger.Memory, store ticket.Store,
	held ticket.Ticket, runID domain.RunID,
) {
	t.Helper()
	started, err := runs.Append(t.Context(), domain.Step{
		RunID: runID, Kind: domain.StepRunStarted,
		Scope: held.Scope, AgentID: held.Agent, VersionID: "v1",
		OnBehalfOf: held.RunAs, Payload: []byte(`{}`), At: ticketNow,
	})
	if err != nil {
		t.Fatalf("append run start: %v", err)
	}
	question, err := runs.Append(t.Context(), domain.Step{
		RunID: runID, Kind: domain.StepApprovalRequested,
		Scope: held.Scope, AgentID: held.Agent, VersionID: "v1",
		OnBehalfOf: held.RunAs,
		Payload:    []byte(`{"tool":"gravitee.accept_subscription","effect":"write"}`),
		At:         started.At.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("append approval: %v", err)
	}
	snapshot := ticket.ContentRef{Ref: "content://snapshot", Digest: "sha256:snapshot"}
	if _, _, err := store.RecordInspection(t.Context(), ticket.InspectionInput{
		Ref: held.Current.Ref, Snapshot: snapshot, At: question.At,
	}); err != nil {
		t.Fatalf("RecordInspection: %v", err)
	}
	if _, _, err := store.AwaitApproval(t.Context(), ticket.ApprovalInput{
		Ref: held.Current.Ref, RunID: runID, AtSeq: question.Seq,
		Snapshot: snapshot, At: question.At,
	}); err != nil {
		t.Fatalf("AwaitApproval: %v", err)
	}
}
