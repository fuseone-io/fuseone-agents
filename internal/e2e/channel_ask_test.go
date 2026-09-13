package e2e_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/channel/connect"
	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/ticket"
	"github.com/fuseone/agents/internal/trigger"
	"github.com/fuseone/agents/internal/vault"
)

/*
A mention in a conversation, all the way to a run (NT-005 stage 3).

Five pieces built separately and never once exercised together: the door that
verifies and writes down, the inbox that holds the ask across a crash, the
consumer that reads who asked and what they asked for, the opener that honours
the same pauses a schedule does, and the driver that says why when the answer
is no.

Each has its own suite and every one of them passed while the path did not
exist. This is the test that would have failed.
*/

const askable = `---
id: helper
name: Helper
area: cx
provider: openai
model: test-model
tools:
  - crm.lookup
budget:
  micros: 500000
  steps: 60
triggers:
  - { type: channel }
---

Answer what you are asked.
`

// conversing is the installation an ask arrives at, configured the way an
// administrator configures it: through the administration area, never by
// writing rows.
type conversing struct {
	pool     *pgxpool.Pool
	store    *settings.Store
	channels *admin.Channels
	registry *spec.Registry
	door     *httptest.Server
	consumer *channel.Consumer
	said     *saidAloud
	version  domain.VersionID
	tickets  *ticket.Postgres
	content  *ledger.Content
}

// saidAloud stands in for the vendor, and records rather than posts.
type saidAloud struct {
	texts   []string
	replies []saidReply
}

type saidReply struct {
	channel      string
	conversation string
	thread       string
	text         string
	outcome      bool
}

func (s *saidAloud) Reply(_ context.Context, channel, conversation, thread, text string) error {
	s.texts = append(s.texts, text)
	s.replies = append(s.replies, saidReply{
		channel: channel, conversation: conversation, thread: thread, text: text,
	})
	return nil
}

func (s *saidAloud) ReplyOutcome(_ context.Context, channel, conversation, thread, text string) error {
	s.texts = append(s.texts, text)
	s.replies = append(s.replies, saidReply{
		channel: channel, conversation: conversation, thread: thread, text: text, outcome: true,
	})
	return nil
}

const signing = "8f742231b10e8888abcd99yyyzzz85a5"

func aConversation(t *testing.T) *conversing {
	t.Helper()
	ctx := context.Background()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping the channel path")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	/*
		The runs go too, and that is not tidiness.

		An ask's idempotency key is derived from the delivery, and these
		deliveries have fixed identifiers. Left behind, the second run of this
		file finds the run the first one opened and answers with it: the sweep
		still reports one, the inbox still says opened, and every assertion
		here still passes — while what was exercised is replay, not opening.
		A test that proves a different thing on its second run proves neither.
	*/
	if _, err := pool.Exec(ctx, `truncate governed_external_attempts,
		governed_ticket_events, governed_ticket_revisions, governed_tickets;
		delete from channel_inbox;
		delete from settings where kind like 'channel%';
		truncate agent_specs; truncate agent_state;
		truncate run_steps, runs, run_content`); err != nil {
		t.Fatalf("clean: %v", err)
	}

	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	store := settings.NewStore(pool, v)
	c := &conversing{
		pool: pool, store: store,
		channels: admin.NewChannels(pool, store, connect.New(store)),
		registry: spec.NewRegistry(pool),
		said:     &saidAloud{},
		tickets:  ticket.NewPostgres(pool),
		content:  ledger.NewContent(pool),
	}

	// Configured through the administration area, which is what records it.
	if err := c.channels.PutChannel(ctx, admin.ChannelWrite{
		Channel: admin.Channel{
			Name: "acme", Kind: "slack", Workspace: "Acme", Enabled: true,
		},
		Credentials: channel.Credentials{Token: "xoxb-acme", Signing: signing},
		By:          "usr_ana", Governs: true,
	}); err != nil {
		t.Fatalf("configure the channel: %v", err)
	}
	if err := c.channels.PutConversation(ctx, "acme", admin.Conversation{
		ID: "C07", Label: "#cx", Enabled: true,
		Scope: domain.Scope{Company: "acme", Area: "cx"},
	}, "usr_ana"); err != nil {
		t.Fatalf("map the conversation: %v", err)
	}
	if err := c.channels.BindIdentity(ctx, admin.ChannelIdentity{
		Channel: "acme", Account: "U505", Principal: "usr_ana",
	}, "usr_ana"); err != nil {
		t.Fatalf("bind the account: %v", err)
	}

	s, err := spec.Parse("helper.agent.md", []byte(askable))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := c.registry.Publish(ctx, s, "usr_ana", "acme"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	c.version = s.Version

	/*
		Let out of Draft and started, the way an operator does it.

		Both are refusals until somebody decides otherwise, and neither has a
		permissive default: an absent state row reads as paused, and an unset
		stage reads as draft. An agent must never begin running because a write
		failed, and a channel is not an exception to that — the ask arrives
		from outside, which is the case it matters most in.
	*/
	state := spec.NewState(pool)
	if err := state.SetStage(ctx, "helper", domain.StageCopilot, "usr_ana"); err != nil {
		t.Fatalf("let the agent out of draft: %v", err)
	}
	if err := state.SetPaused(ctx, "helper", false, "usr_ana", time.Now()); err != nil {
		t.Fatalf("start the agent: %v", err)
	}

	// The door: the secret that says a request is genuine, and who the account
	// it names speaks for. It cannot configure anything.
	hooks := httpapi.NewChannelHooks(
		nil, admin.NewChannelDoor(pool, store), nil, time.Now, slog.Default()).
		WithArrivals(channel.NewInbox(pool)).
		WithTicketRoutes(channel.NewTicketRoutes(store, c.tickets))
	mux := http.NewServeMux()
	hooks.MountEvents(mux)
	c.door = httptest.NewServer(mux)
	t.Cleanup(c.door.Close)

	opener := trigger.NewOpener(ledger.NewPostgres(pool), c.registry, engine.SystemClock{}).
		WithContent(ledger.NewContent(pool)).
		WithPauses(spec.NewState(pool)).
		WithStops(admin.NewStops(pool)).
		WithStages(spec.NewState(pool))

	c.consumer = channel.NewConsumer(channel.NewInbox(pool), "test-asks", slog.Default()).
		With(
			channel.NewConfigured(store), c.registry, c.registry,
			channel.NewPostgres(pool), channel.FromTrigger(opener), c.said,
		).
		WithOutcomes(channel.NewPostgres(pool), c.content).
		Binding(c.channels.PrincipalFor).
		WithTickets(channel.NewTicketHandler(
			c.tickets, c.content, channel.FromTrigger(opener),
			admin.NewChannelFacts(pool, store), admin.NewTicketAddresses(store),
			auth.NewPostgres(pool), ledger.NewPostgres(pool), time.Now,
		))
	return c
}

// mention posts a signed app_mention, the way Slack does.
func (c *conversing) mention(t *testing.T, eventID, text string) *http.Response {
	t.Helper()
	return c.event(t, eventID, map[string]any{
		"type": "app_mention", "channel": "C07", "user": "U505",
		"text": text, "ts": "1786.1",
	})
}

func (c *conversing) event(t *testing.T, eventID string, event map[string]any) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"type": "event_callback", "event_id": eventID, "event": event,
	})

	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(signing))
	fmt.Fprintf(mac, "v0:%s:%s", stamp, body)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		c.door.URL+"/hooks/channel/acme/slack/events", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Slack-Request-Timestamp", stamp)
	req.Header.Set("X-Slack-Signature", "v0="+hex.EncodeToString(mac.Sum(nil)))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func (c *conversing) ticketRoot(t *testing.T, eventID, text, at string) *http.Response {
	t.Helper()
	return c.event(t, eventID, map[string]any{
		"type": "message", "channel": "C07", "user": "U505",
		"text": text, "ts": at,
	})
}

// opened answers what the inbox recorded for one delivery.
func (c *conversing) opened(t *testing.T, eventID string) (status, runID, detail string) {
	t.Helper()
	if err := c.pool.QueryRow(context.Background(),
		`select status, run_id, detail from channel_inbox where event_id = $1`,
		eventID).Scan(&status, &runID, &detail); err != nil {
		t.Fatalf("read the inbox: %v", err)
	}
	return status, runID, detail
}

// runs is how many the ledger holds, which is how this tells opening a run
// apart from being handed one that was already there.
func (c *conversing) runs(t *testing.T) int {
	t.Helper()
	var n int
	if err := c.pool.QueryRow(context.Background(),
		`select count(*) from runs`).Scan(&n); err != nil {
		t.Fatalf("count the runs: %v", err)
	}
	return n
}

func (c *conversing) finish(t *testing.T, runID string, outcome string) {
	t.Helper()
	ctx := context.Background()

	content := ledger.NewContent(c.pool)
	ref, err := content.Put(ctx, domain.RunID(runID), 2, []byte(outcome))
	if err != nil {
		t.Fatalf("store the outcome: %v", err)
	}

	steps, err := ledger.NewPostgres(c.pool).Read(ctx, domain.RunID(runID), 0)
	if err != nil {
		t.Fatalf("read the opened run: %v", err)
	}
	first := steps[0]
	payload, _ := json.Marshal(domain.RunFinishedPayload{
		OutcomeRef: ref, OutcomeDigest: "sha256:test",
	})
	if _, err := ledger.NewPostgres(c.pool).Append(ctx, domain.Step{
		RunID: domain.RunID(runID), Kind: domain.StepRunFinished,
		Scope: first.Scope, AgentID: first.AgentID, VersionID: first.VersionID,
		OnBehalfOf: first.OnBehalfOf, Payload: payload, At: time.Now(),
	}); err != nil {
		t.Fatalf("finish the run: %v", err)
	}
}

func TestAsk_aMentionInAMappedConversation_becomesARunForWhoeverAsked(t *testing.T) {
	c := aConversation(t)

	if resp := c.mention(t, "Ev1", "<@U0BOT> helper look at the queue"); resp.StatusCode != http.StatusOK {
		t.Fatalf("the door answered %d; Slack would retry a question it already has", resp.StatusCode)
	}

	// Nothing has run yet. The door writes and acknowledges, and that is all.
	if status, _, _ := c.opened(t, "Ev1"); status != "pending" {
		t.Fatalf("status = %q before any sweep, want the ask waiting", status)
	}

	before := c.runs(t)
	n, err := c.consumer.Sweep(context.Background(), time.Minute, 10)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n != 1 {
		t.Fatalf("opened %d, want the one ask that arrived", n)
	}
	// Opened, not found. The sweep counts asks it settled, and an ask settled
	// by the opener handing back a run it already had counts the same — which
	// is the whole of what this test would silently stop proving.
	if got := c.runs(t); got != before+1 {
		t.Fatalf("runs went %d to %d; the ask did not open one", before, got)
	}

	status, runID, detail := c.opened(t, "Ev1")
	if status != "opened" || runID == "" {
		t.Fatalf("status = %q run = %q detail = %q", status, runID, detail)
	}

	// The run acts for the person bound to the account, in the area the
	// conversation speaks for, and remembers where it was asked.
	steps, err := ledger.NewPostgres(c.pool).Read(context.Background(), domain.RunID(runID), 0)
	if err != nil {
		t.Fatalf("read the run: %v", err)
	}
	first := steps[0]
	if first.Scope.Area != "cx" {
		t.Errorf("area = %q, want the scope the conversation speaks for", first.Scope.Area)
	}
	if first.OnBehalfOf != "usr_ana" {
		t.Errorf("on behalf of %q, want the person the account is bound to", first.OnBehalfOf)
	}
	var started domain.RunStartedPayload
	if err := json.Unmarshal(first.Payload, &started); err != nil {
		t.Fatalf("read what started it: %v", err)
	}
	if started.Origin == nil || started.Origin.Conversation != "C07" {
		t.Errorf("origin = %+v, want the conversation it was asked in", started.Origin)
	}
	if len(c.said.texts) != 0 {
		t.Errorf("said %v; opening a run says nothing, the run does", c.said.texts)
	}
}

func TestAsk_aFinishedRunAnswersInTheThreadThatAsked(t *testing.T) {
	c := aConversation(t)

	c.mention(t, "EvAnswer", "<@U0BOT> helper diagnose this alert")
	if _, err := c.consumer.Sweep(context.Background(), time.Minute, 10); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	_, runID, _ := c.opened(t, "EvAnswer")
	c.finish(t, runID, "diagnosis complete")

	if len(c.said.texts) != 0 {
		t.Fatalf("said before answer sweep: %v", c.said.texts)
	}
	delivered, err := c.consumer.AnswerFinished(context.Background(), time.Minute, 10)
	if err != nil {
		t.Fatalf("answer finished: %v", err)
	}
	if delivered != 1 || len(c.said.replies) != 1 {
		t.Fatalf("delivered %d replies %+v", delivered, c.said.replies)
	}
	got := c.said.replies[0]
	if got.channel != "acme" || got.conversation != "C07" || got.thread != "1786.1" {
		t.Fatalf("reply went to %s/%s/%s, want the original channel conversation and thread",
			got.channel, got.conversation, got.thread)
	}
	if !got.outcome {
		t.Fatal("finished answer used the literal refusal path; it should use the vendor's outcome renderer")
	}
	if got.text != "diagnosis complete" {
		t.Errorf("text = %q", got.text)
	}

	again, err := c.consumer.AnswerFinished(context.Background(), time.Minute, 10)
	if err != nil || again != 0 {
		t.Fatalf("delivered %d more (%v); the answer was already said", again, err)
	}
}

/*
A mention naming an agent nobody published is answered, not swallowed.

The second time somebody is ignored they stop asking, so the refusal is
recorded by the consumer that holds the claim and delivered by its own sweep —
and until that sweep runs, nobody has been told.
*/
func TestAsk_namingAnAgentThatCannotBeStarted_isRefusedInTheThread(t *testing.T) {
	c := aConversation(t)
	c.mention(t, "Ev2", "<@U0BOT> nonesuch do the thing")

	if _, err := c.consumer.Sweep(context.Background(), time.Minute, 10); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	status, runID, detail := c.opened(t, "Ev2")
	if status != "refused" || runID != "" {
		t.Fatalf("status = %q run = %q, want a refusal and no run", status, runID)
	}
	if detail == "" {
		t.Fatal("no reason recorded; an operator asked why nothing happened has nothing to read")
	}
	if len(c.said.texts) != 0 {
		t.Fatalf("said %v during the sweep; the reply is owed, not sent", c.said.texts)
	}

	said, err := c.consumer.Answer(context.Background(), time.Minute, 10)
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if said != 1 || len(c.said.texts) != 1 {
		t.Fatalf("delivered %d, texts %v", said, c.said.texts)
	}

	// And it is not owed twice. A refusal repeated every ten seconds is a
	// worse conversation than one that never came.
	again, err := c.consumer.Answer(context.Background(), time.Minute, 10)
	if err != nil || again != 0 {
		t.Fatalf("delivered %d more (%v); the refusal was already said", again, err)
	}
}

/*
Slack retries what it does not get, and a retry is the same question.

The door answers 200 to both and the consumer sees one ask: the pair the
inbox is keyed by is the sender's own, because the sender is the only party who
knows that two deliveries are the same delivery.
*/
func TestAsk_theSameDeliveryTwice_opensOneRun(t *testing.T) {
	c := aConversation(t)
	c.mention(t, "Ev3", "<@U0BOT> helper look at the queue")
	c.mention(t, "Ev3", "<@U0BOT> helper look at the queue")

	n, err := c.consumer.Sweep(context.Background(), time.Minute, 10)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n != 1 {
		t.Fatalf("opened %d, want one run for one question asked once", n)
	}
	if got := c.runs(t); got != 1 {
		t.Fatalf("runs = %d, want the single run the retried question became", got)
	}
}

func TestGovernedTicket_aSignedRootBecomesOneSafeGraviteeOutcome(t *testing.T) {
	c := aConversation(t)
	ctx := context.Background()
	if err := c.channels.PutConversation(ctx, "acme", admin.Conversation{
		ID: "C07", Label: "#api-access", Enabled: true,
		Scope: domain.Scope{Company: "acme", Area: "cx"},
		Mode:  channel.ConversationTicket, Agent: "helper", RunAs: "usr_ana",
		Ticket: &channel.TicketRule{
			OpenFrom: channel.TicketOpenLinkedUsers, AddressFrom: "app:A-TICKET",
			Patterns: []string{`\bapi[ -]?key\b`},
		},
	}, "usr_ana"); err != nil {
		t.Fatalf("configure governed tickets: %v", err)
	}

	if resp := c.ticketRoot(t, "EvTicket", "Please approve API key sub-42 until Friday", "1787.1"); resp.StatusCode != http.StatusOK {
		t.Fatalf("ticket door answered %d", resp.StatusCode)
	}
	if opened, err := c.consumer.Sweep(ctx, time.Minute, 10); err != nil || opened != 1 {
		t.Fatalf("ticket sweep = (%d, %v)", opened, err)
	}
	key, err := ticket.Key("acme", "C07", "1787.1")
	if err != nil {
		t.Fatal(err)
	}
	held, err := c.tickets.Current(ctx, key)
	if err != nil {
		t.Fatalf("read governed ticket: %v", err)
	}
	if held.Agent != "helper" || held.RunAs != "usr_ana" || held.RequestedBy != "usr_ana" ||
		held.Origin != (ticket.Origin{Connection: "acme", Conversation: "C07", Root: "1787.1"}) {
		t.Fatalf("ticket identity = %+v", held)
	}

	rawDraft, err := c.content.Get(ctx, held.Current.Draft.Ref)
	if err != nil {
		t.Fatalf("read ticket draft: %v", err)
	}
	for _, forbidden := range []string{"U505", "channel", "event_id", "A-TICKET"} {
		if bytes.Contains(rawDraft, []byte(forbidden)) {
			t.Fatalf("canonical draft exposed Slack envelope field %q: %s", forbidden, rawDraft)
		}
	}
	if !bytes.Contains(rawDraft, []byte("Please approve API key sub-42")) ||
		!bytes.Contains(rawDraft, []byte("usr_ana")) {
		t.Fatalf("canonical draft lost requester text or identity: %s", rawDraft)
	}

	_, runID, _ := c.opened(t, "EvTicket")
	steps, err := ledger.NewPostgres(c.pool).Read(ctx, domain.RunID(runID), 0)
	if err != nil || len(steps) == 0 {
		t.Fatalf("read ticket run = (%d steps, %v)", len(steps), err)
	}
	var started domain.RunStartedPayload
	if err := json.Unmarshal(steps[0].Payload, &started); err != nil || started.Ticket == nil ||
		started.Ticket.Ref != held.Current.Ref || started.Ticket.RequestedBy != "usr_ana" ||
		steps[0].OnBehalfOf != "usr_ana" {
		t.Fatalf("sealed ticket run = (%+v, %+v, %v)", steps[0], started, err)
	}

	expires := "2026-09-30T18:00:00Z"
	application := connectortools.GraviteeApplication{
		GraviteeResource:  connectortools.GraviteeResource{ID: "app-1", Name: "Payments"},
		PrimaryOwnerEmail: "ana@example.com",
	}
	api := connectortools.GraviteeResource{ID: "checkout-api", Name: "Checkout"}
	plan := connectortools.GraviteeResource{ID: "plan-1", Name: "API Key"}
	snapshotValue := connectortools.GraviteeSnapshot{
		TicketRef: held.Current.Ref, SubscriptionID: "sub-42", Status: "PENDING",
		Application: application, API: api, Plan: plan, PlanSecurity: "API_KEY",
		RequestedExpiration: &expires,
		RemoteCreatedAt:     "2026-09-10T12:00:00Z", RemoteUpdatedAt: "2026-09-11T12:00:00Z",
	}
	snapshotRaw, err := json.Marshal(snapshotValue)
	if err != nil {
		t.Fatal(err)
	}
	snapshotRef, err := c.content.Put(ctx, domain.RunID(runID), 2, snapshotRaw)
	if err != nil {
		t.Fatalf("store snapshot: %v", err)
	}
	snapshot := ticket.ContentRef{Ref: snapshotRef, Digest: engine.ResultDigest(snapshotRaw)}
	if _, _, err := c.tickets.RecordInspection(ctx, ticket.InspectionInput{
		Ref: held.Current.Ref, Snapshot: snapshot, At: time.Now(),
	}); err != nil {
		t.Fatalf("record inspection: %v", err)
	}
	evidence := domain.ApprovalEvidence{
		Kind:   connectortools.ApprovalEvidenceGraviteeSubscription,
		Ticket: held.Current.Ref, Ref: snapshot.Ref, Digest: snapshot.Digest,
	}
	store := ledger.NewPostgres(c.pool)
	approvalPayload, _ := json.Marshal(domain.ApprovalRequestedPayload{
		Tool: "gravitee.apim.accept_subscription", Rule: "explicit_approval",
		Effect: domain.EffectWrite, Evidence: &evidence,
	})
	approvalStep, err := store.Append(ctx, domain.Step{
		RunID: domain.RunID(runID), Kind: domain.StepApprovalRequested,
		Scope: held.Scope, AgentID: held.Agent, VersionID: c.version,
		OnBehalfOf: held.RequestedBy, Payload: approvalPayload, At: time.Now(),
	})
	if err != nil {
		t.Fatalf("append approval request: %v", err)
	}
	if _, _, err := c.tickets.AwaitApproval(ctx, ticket.ApprovalInput{
		Ref: held.Current.Ref, RunID: domain.RunID(runID), AtSeq: approvalStep.Seq,
		Snapshot: snapshot, At: time.Now(),
	}); err != nil {
		t.Fatalf("record approval request: %v", err)
	}
	decidedPayload, _ := json.Marshal(domain.ApprovalDecidedPayload{
		Approved: true, By: "usr_manager", AtSeq: approvalStep.Seq,
	})
	if _, err := store.AppendIfHead(ctx,
		domain.StepRef{Seq: approvalStep.Seq, Kind: domain.StepApprovalRequested},
		domain.Step{
			RunID: domain.RunID(runID), Kind: domain.StepApprovalDecided,
			Scope: held.Scope, AgentID: held.Agent, VersionID: c.version,
			OnBehalfOf: held.RequestedBy, Payload: decidedPayload,
			IdemKey: domain.ApprovalDecisionKey(domain.RunID(runID), approvalStep.Seq), At: time.Now(),
		}); err != nil {
		t.Fatalf("append approval decision: %v", err)
	}
	gravitee := connectortools.Instance{
		Connector: "gravitee", Name: "apim", Scope: held.Scope, Enabled: true,
		Gravitee: connectortools.GraviteeConfig{
			Address:      "https://gravitee.example/management/v2",
			Organization: "org-prod", Environment: "env-prod",
			AllowedReferences: []connectortools.GraviteeReference{{
				Type: connectortools.GraviteeReferenceAPI, ID: "checkout-api",
			}},
			MinTTLSeconds: 3600, MaxTTLSeconds: 90 * 24 * 60 * 60,
			CredentialSource: connectortools.GraviteeCredentialSource{
				Kind: connectortools.GraviteeCredentialVaultKV, VaultInstance: "secrets",
				Path: "integrations/gravitee/prod", Field: "access_token",
			},
		},
	}
	vaultInstance := connectortools.Instance{
		Connector: "vault", Name: "secrets", Scope: held.Scope, Enabled: true,
		Vault: connectortools.VaultConfig{
			Address: "https://vault.example", Mount: "secret",
			AllowedPathPrefixes: []string{"integrations/gravitee"},
		},
	}
	remote := &e2eGraviteeRemote{
		inspected: connectortools.GraviteeObservation{
			SubscriptionID: "sub-42", Status: "PENDING", Application: application,
			API: api, Plan: plan, PlanSecurity: "API_KEY",
			CreatedAt: snapshotValue.RemoteCreatedAt, UpdatedAt: snapshotValue.RemoteUpdatedAt,
		},
		accepted: connectortools.GraviteeObservation{
			SubscriptionID: "sub-42", Status: "ACCEPTED", Application: application,
			API: api, Plan: plan, PlanSecurity: "API_KEY", EndingAt: &expires,
			CreatedAt: snapshotValue.RemoteCreatedAt, UpdatedAt: "2026-09-12T12:00:00Z",
		},
	}
	attempts := connectortools.NewPostgresGraviteeAttempts(c.pool)
	access := &e2eGraviteeAccess{config: gravitee.Gravitee}
	runtime := connectortools.NewGraviteeAcceptRuntime(
		access, remote, c.content, c.tickets, attempts,
		connectortools.NewGraviteeReconciliationLedger(store),
	)
	layer := connectortools.New(nil, nil, c.content, nil).WithGraviteeRuntime(runtime)
	if err := layer.SetInstances([]connectortools.Instance{gravitee, vaultInstance}); err != nil {
		t.Fatalf("configure native layer: %v", err)
	}
	args, _ := json.Marshal(connectortools.GraviteeInspectInput{
		SubscriptionID: "sub-42", ExpiresAt: expires,
	})
	call := engine.Call{
		RunID: domain.RunID(runID), Tool: "gravitee.apim.accept_subscription",
		Scope: held.Scope, Args: args, Ticket: *started.Ticket,
		ApprovalEvidence: evidence, ApprovalAtSeq: approvalStep.Seq,
		DecidedBy: "usr_manager", IdemKey: "e2e-gravitee-accept", At: time.Now(),
	}
	call.ContractDigest = layer.ApprovalBinding(call)
	access.contract = call.ContractDigest
	if err := layer.Reserve(ctx, call); err != nil {
		t.Fatalf("reserve Gravitee acceptance: %v", err)
	}
	calledPayload, _ := json.Marshal(domain.ToolCalledPayload{
		Tool: call.Tool, Effect: domain.EffectWrite, ContractDigest: call.ContractDigest,
	})
	called, err := store.Append(ctx, domain.Step{
		RunID: domain.RunID(runID), Kind: domain.StepToolCalled,
		Scope: held.Scope, AgentID: held.Agent, VersionID: c.version,
		OnBehalfOf: held.RequestedBy, IdemKey: call.IdemKey,
		Payload: calledPayload, At: call.At,
	})
	if err != nil {
		t.Fatalf("append tool call: %v", err)
	}
	call.Seq = called.Seq
	result, err := layer.Invoke(ctx, call)
	if err != nil || result.Failed || remote.acceptCalls != 1 {
		t.Fatalf("native acceptance = (%+v, %v), POST calls=%d", result, err, remote.acceptCalls)
	}
	steps, _ = store.Read(ctx, domain.RunID(runID), domain.FirstSeq)
	if countStep(steps, domain.StepToolReturned) != 0 || countStep(steps, domain.StepEffectReconciled) != 0 {
		t.Fatalf("the simulated dead process sealed a result: %v", stepKinds(steps))
	}

	// The process dies here: the remote result and durable attempt exist, but
	// the engine never appends tool_returned. Make the journal due and compose
	// a fresh runtime, as a restarted worker would.
	if _, err := c.pool.Exec(ctx, `update governed_external_attempts
		set next_check_at = now() - interval '1 second', claimed_by = '', claimed_until = null
		where idem_key = $1`, call.IdemKey); err != nil {
		t.Fatalf("age the abandoned attempt: %v", err)
	}
	recovered := connectortools.NewGraviteeAcceptRuntime(
		access, remote, c.content, c.tickets, attempts,
		connectortools.NewGraviteeReconciliationLedger(store),
	)
	if count, err := connectortools.NewGraviteeAttemptReconciler(
		recovered, attempts, "e2e-restarted-worker").Sweep(ctx); err != nil || count != 1 {
		t.Fatalf("reconcile abandoned acceptance = (%d, %v)", count, err)
	}
	if remote.acceptCalls != 1 {
		t.Fatalf("POST calls after reconciliation = %d, want exactly one", remote.acceptCalls)
	}
	steps, _ = store.Read(ctx, domain.RunID(runID), domain.FirstSeq)
	if countStep(steps, domain.StepEffectReconciled) != 1 ||
		countStep(steps, domain.StepToolReturned) != 0 {
		t.Fatalf("reconciled trail = %v", stepKinds(steps))
	}

	outcomes := channel.NewTicketOutcomeConsumer(
		c.tickets, c.content, c.said, connectortools.GraviteeTicketOutcomeRenderer{}, "e2e-ticket",
	)
	if delivered, err := outcomes.Sweep(ctx, time.Minute, 10); err != nil || delivered != 1 {
		t.Fatalf("deliver ticket outcome = (%d, %v)", delivered, err)
	}
	got := c.said.replies[len(c.said.replies)-1]
	if got.channel != "acme" || got.conversation != "C07" || got.thread != "1787.1" || !got.outcome {
		t.Fatalf("ticket outcome destination = %+v", got)
	}
	for _, want := range []string{"approved in Gravitee", expires, "never posts the API key", "revoke or rotate"} {
		if !strings.Contains(got.text, want) {
			t.Errorf("ticket outcome = %q; missing %q", got.text, want)
		}
	}
	if strings.Contains(got.text, "api-key-canary") {
		t.Fatalf("ticket outcome exposed an API key: %q", got.text)
	}
	if delivered, err := outcomes.Sweep(ctx, time.Minute, 10); err != nil || delivered != 0 {
		t.Fatalf("ticket outcome repeated = (%d, %v)", delivered, err)
	}
}

type e2eGraviteeAccess struct {
	config   connectortools.GraviteeConfig
	contract string
}

func (a *e2eGraviteeAccess) Resolve(
	context.Context, string, domain.Scope,
) (connectortools.GraviteeAccess, error) {
	return connectortools.GraviteeAccess{Config: a.config, ContractDigest: a.contract}, nil
}

type e2eGraviteeRemote struct {
	inspected    connectortools.GraviteeObservation
	accepted     connectortools.GraviteeObservation
	inspectCalls int
	observeCalls int
	acceptCalls  int
}

func (r *e2eGraviteeRemote) Inspect(
	context.Context, connectortools.GraviteeConfig, connectortools.SecretValue, string,
) (connectortools.GraviteeObservation, error) {
	r.inspectCalls++
	return r.inspected, nil
}

func (r *e2eGraviteeRemote) Observe(
	context.Context, connectortools.GraviteeConfig, connectortools.SecretValue, string,
) (connectortools.GraviteeObservation, error) {
	r.observeCalls++
	return r.accepted, nil
}

func (r *e2eGraviteeRemote) Accept(
	context.Context, connectortools.GraviteeConfig, connectortools.SecretValue,
	connectortools.GraviteeSnapshot,
) (connectortools.GraviteeObservation, error) {
	r.acceptCalls++
	return r.accepted, nil
}

func countStep(steps []domain.Step, kind domain.StepKind) int {
	count := 0
	for _, step := range steps {
		if step.Kind == kind {
			count++
		}
	}
	return count
}

func stepKinds(steps []domain.Step) []domain.StepKind {
	out := make([]domain.StepKind, len(steps))
	for i, step := range steps {
		out[i] = step.Kind
	}
	return out
}
