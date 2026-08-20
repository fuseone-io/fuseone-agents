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
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/spec"
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
	if _, err := pool.Exec(ctx, `delete from channel_inbox;
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
		channels: admin.NewChannels(pool, store),
		registry: spec.NewRegistry(pool),
		said:     &saidAloud{},
	}

	// Configured through the administration area, which is what records it.
	if err := c.channels.PutChannel(ctx,
		admin.Channel{Name: "acme", Kind: "slack", Workspace: "Acme", Enabled: true},
		channel.Credentials{Token: "xoxb-acme", Signing: signing}, "usr_ana"); err != nil {
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

	hooks := httpapi.NewChannelHooks(nil, c.channels, nil, time.Now, slog.Default()).
		WithArrivals(channel.NewInbox(pool))
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
		WithOutcomes(channel.NewPostgres(pool), ledger.NewContent(pool)).
		Binding(c.channels.PrincipalFor)
	return c
}

// mention posts a signed app_mention, the way Slack does.
func (c *conversing) mention(t *testing.T, eventID, text string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"type": "event_callback", "event_id": eventID,
		"event": map[string]any{
			"type": "app_mention", "channel": "C07", "user": "U505",
			"text": text, "ts": "1786.1",
		},
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
