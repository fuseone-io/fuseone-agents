package slack_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/channel/slack"
	"github.com/fuseone/agents/internal/domain"
)

func TestSocketReceiver_acknowledgesOnlyAfterTheAskIsRecorded(t *testing.T) {
	t.Parallel()
	inbox := &socketInbox{}

	ack, err := (slack.SocketReceiver{
		Channel: "cora-slack",
		Inbox:   inbox,
	}).Handle(context.Background(), socketFrame("env-1", slackEvent()))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if string(ack) != `{"envelope_id":"env-1"}` {
		t.Fatalf("ack = %s", ack)
	}
	if inbox.got.Channel != "cora-slack" ||
		inbox.got.Conversation != "C07" ||
		inbox.got.EventID != "Ev1" ||
		inbox.got.Message != "1786.42" ||
		inbox.got.Thread != "1786.42" ||
		inbox.got.AskedBy != "U505" {
		t.Fatalf("arrival = %+v", inbox.got)
	}
}

func TestSocketReceiver_aMentionMarksTheSlackAccountAsSeen(t *testing.T) {
	t.Parallel()
	inbox := &socketInbox{}
	seen := &socketSeen{}

	ack, err := (slack.SocketReceiver{
		Channel: "cora-slack",
		Inbox:   inbox,
		Seen:    seen,
	}).Handle(context.Background(), socketFrame("env-seen", slackEvent()))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if string(ack) != `{"envelope_id":"env-seen"}` {
		t.Fatalf("ack = %s", ack)
	}
	if seen.channel != "cora-slack" || seen.account != "U505" || seen.conversation != "C07" {
		t.Fatalf("seen = %s/%s/%s, want the signed Slack account", seen.channel, seen.account, seen.conversation)
	}
	if inbox.got.EventID != "Ev1" {
		t.Fatalf("arrival = %+v, want the ask still written", inbox.got)
	}
}

func TestSocketReceiver_whenTheInboxCannotRecord_doesNotAcknowledge(t *testing.T) {
	t.Parallel()
	inbox := &socketInbox{err: errors.New("postgres is down")}

	ack, err := (slack.SocketReceiver{
		Channel: "cora-slack",
		Inbox:   inbox,
	}).Handle(context.Background(), socketFrame("env-1", slackEvent()))
	if err == nil {
		t.Fatal("Handle succeeded; Slack would stop retrying an ask we lost")
	}
	if ack != nil {
		t.Fatalf("ack = %s, want none", ack)
	}
}

func TestSocketReceiver_ordinarySocketMessagesAreAcknowledgedAndIgnored(t *testing.T) {
	t.Parallel()
	ack, err := (slack.SocketReceiver{
		Channel: "cora-slack",
		Inbox:   &socketInbox{},
	}).Handle(context.Background(), socketFrame("env-1", map[string]any{
		"type":     "event_callback",
		"event_id": "Ev1",
		"event":    map[string]any{"type": "message"},
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if string(ack) != `{"envelope_id":"env-1"}` {
		t.Fatalf("ack = %s", ack)
	}
}

func TestSocketReceiver_aWatchedSourceRecordsTheConfiguredAutomation(t *testing.T) {
	t.Parallel()
	inbox := &socketInbox{}

	ack, err := (slack.SocketReceiver{
		Channel: "cora-slack",
		Inbox:   inbox,
		Rules: socketRules{
			source: "B-alerts", agent: "troubleshooting-sre", runAs: "usr_opsbot",
		},
	}).Handle(context.Background(), socketFrame("env-2", map[string]any{
		"type":     "event_callback",
		"event_id": "Ev-alert",
		"event": map[string]any{
			"type": "message", "subtype": "bot_message",
			"channel": "C07", "bot_id": "B-alerts",
			"text": "firing alertGatewayRTMInterfaceErrors", "ts": "1786.50",
		},
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if string(ack) != `{"envelope_id":"env-2"}` {
		t.Fatalf("ack = %s", ack)
	}
	if inbox.got.Agent != "troubleshooting-sre" ||
		inbox.got.RunAs != "usr_opsbot" ||
		inbox.got.AskedBy != "bot:B-alerts" {
		t.Fatalf("arrival = %+v, want configured automation", inbox.got)
	}
}

func TestSocketReceiver_aMessageFromAnotherSourceDoesNotEnterTheInbox(t *testing.T) {
	t.Parallel()
	inbox := &socketInbox{}

	ack, err := (slack.SocketReceiver{
		Channel: "cora-slack",
		Inbox:   inbox,
		Rules: socketRules{
			source: "B-alerts", agent: "troubleshooting-sre", runAs: "usr_opsbot",
		},
	}).Handle(context.Background(), socketFrame("env-3", map[string]any{
		"type":     "event_callback",
		"event_id": "Ev-noise",
		"event": map[string]any{
			"type": "message", "subtype": "bot_message",
			"channel": "C07", "bot_id": "B-other",
			"text": "noise", "ts": "1786.51",
		},
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if string(ack) != `{"envelope_id":"env-3"}` {
		t.Fatalf("ack = %s", ack)
	}
	if inbox.got.EventID != "" {
		t.Fatalf("arrival = %+v, want nothing written", inbox.got)
	}
}

func TestOpenSocketURL_usesTheAppLevelTokenAsABearer(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer xapp-secret" {
			t.Fatalf("authorization = %q", got)
		}
		if r.URL.Path != "/apps.connections.open" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true,"url":"wss://example.test/link"}`))
	}))
	defer server.Close()

	got, err := slack.OpenSocketURL(context.Background(), "xapp-secret", server.URL, server.Client())
	if err != nil {
		t.Fatalf("OpenSocketURL: %v", err)
	}
	if got != "wss://example.test/link" {
		t.Fatalf("url = %q", got)
	}
}

type socketInbox struct {
	got channel.Arrival
	err error
}

func (i *socketInbox) Receive(
	_ context.Context, a channel.Arrival,
) (bool, error) {
	i.got = a
	return true, i.err
}

type socketSeen struct {
	channel      string
	account      string
	conversation string
}

func (s *socketSeen) MarkAccountSeen(
	_ context.Context, channelName, account, conversation string, _ time.Time,
) error {
	s.channel, s.account, s.conversation = channelName, account, conversation
	return nil
}

type socketRules struct {
	source string
	agent  domain.AgentID
	runAs  domain.UserID
}

func (r socketRules) WatchFor(
	_ context.Context, _, _ string, source channel.Source,
) (channel.WatchRule, bool, error) {
	if !source.Matches([]string{r.source}) {
		return channel.WatchRule{}, false, nil
	}
	return channel.WatchRule{
		Agent: r.agent, RunAs: r.runAs, Sources: []string{r.source},
	}, true, nil
}

func socketFrame(envelope string, payload any) []byte {
	raw, err := json.Marshal(map[string]any{
		"type":        "events_api",
		"envelope_id": envelope,
		"payload":     payload,
	})
	if err != nil {
		panic(err)
	}
	return raw
}

func slackEvent() map[string]any {
	return map[string]any{
		"type":     "event_callback",
		"event_id": "Ev1",
		"event": map[string]any{
			"type":    "app_mention",
			"channel": "C07",
			"user":    "U505",
			"text":    "<@U07BOT> troubleshooting-sre diagnose it",
			"ts":      "1786.42",
		},
	}
}
