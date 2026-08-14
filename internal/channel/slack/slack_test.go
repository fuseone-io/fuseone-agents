package slack_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/channel/slack"
	"github.com/fuseone/agents/internal/domain"
)

/*
Talking to Slack.

Two things this has to get right and neither is the HTTP. Slack answers 200
with `{"ok": false}` for most of what goes wrong — a bot removed from a
channel, a revoked token, a channel that no longer exists — so a driver that
trusts the status code reports success for every one of them, and the platform
learns it is being ignored by never hearing anything again.

The other is that the message body is built here rather than by the caller.
Blocks are Slack's answer to "a heading, some facts and a link"; Teams has a
different one; a caller that knew either would have to know both.
*/
func TestPost_channelAnswersOkFalse_isAFailureNotASuccess(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":false,"error":"not_in_channel"}`)
	}))
	defer server.Close()

	_, err := poster(server).Post(t.Context(),
		channel.Conversation{ID: "C07", Label: "#ops"}, message())
	if err == nil {
		t.Fatal("a 200 carrying ok:false was read as a delivered message")
	}
	if !strings.Contains(err.Error(), "not_in_channel") {
		t.Errorf("err = %v, want Slack's own reason so an operator knows what to fix", err)
	}
}

func TestPost_delivered_returnsTheTimestampAReplyWouldThreadOn(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"ts":"1786000000.000100"}`)
	}))
	defer server.Close()

	// The timestamp is the thread. Stage 2 replies into it, so a driver that
	// dropped it would make the approval land as a new message with no
	// relation to the thing it is about.
	ref, err := poster(server).Post(t.Context(),
		channel.Conversation{ID: "C07", Label: "#ops"}, message())
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if ref != "1786000000.000100" {
		t.Errorf("ref = %q, want the message timestamp", ref)
	}
}

func TestPost_carriesTheTokenAsBearerAndTheChannelInTheBody(t *testing.T) {
	t.Parallel()
	var got struct {
		Channel string            `json:"channel"`
		Text    string            `json:"text"`
		Blocks  []json.RawMessage `json:"blocks"`
	}
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"ts":"1.1"}`)
	}))
	defer server.Close()

	if _, err := poster(server).Post(t.Context(),
		channel.Conversation{ID: "C07", Label: "#ops"}, message()); err != nil {
		t.Fatalf("post: %v", err)
	}

	if auth != "Bearer xoxb-test" {
		t.Errorf("authorization = %q", auth)
	}
	if got.Channel != "C07" {
		t.Errorf("channel = %q", got.Channel)
	}
	// Notifications and accessibility read the fallback text, not the blocks.
	// A message with blocks and no text is silent on a phone.
	if got.Text == "" {
		t.Error("no fallback text; the notification a phone shows would be empty")
	}
	if len(got.Blocks) == 0 {
		t.Error("no blocks; the message would be one unreadable line")
	}
}

// The run identifier and the reason are the two things a reader acts on, so
// they have to survive into the message rather than being summarised away.
func TestPost_messageNamesTheRunAndWhyItStopped(t *testing.T) {
	t.Parallel()
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"ts":"1.1"}`)
	}))
	defer server.Close()

	if _, err := poster(server).Post(t.Context(),
		channel.Conversation{ID: "C07", Label: "#ops"}, message()); err != nil {
		t.Fatalf("post: %v", err)
	}

	for _, want := range []string{"run_ops_1786", "erp.transfer", "https://agents.example.com/runs/run_ops_1786"} {
		if !strings.Contains(body, want) {
			t.Errorf("the message does not carry %q", want)
		}
	}
}

func poster(server *httptest.Server) *slack.Poster {
	return slack.New("xoxb-test").WithEndpointBase(server.URL)
}

func message() channel.Message {
	return channel.Message{
		Event: channel.EventParked,
		RunID: "run_ops_1786",
		Agent: "triage",
		Tool:  "erp.transfer",
		Link:  "https://agents.example.com/runs/run_ops_1786",
	}
}

/*
Drift is not a run, and the message carrying it has no run to link to.

Rendered through the default arm it would read as "triage finished", which
tells a reader the opposite of what happened — and this is the one notice that
fires when nobody is looking for it.
*/
func TestPost_drift_saysNothingWasPublishedAndNamesTheCorrection(t *testing.T) {
	t.Parallel()

	var body struct {
		Blocks []struct {
			Text struct {
				Text string `json:"text"`
			} `json:"text"`
		} `json:"blocks"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"ts":"1"}`)
	}))
	defer server.Close()

	_, err := poster(server).Post(t.Context(),
		channel.Conversation{ID: "C07", Label: "#ops"},
		channel.Message{
			Event: channel.EventDrifted, Agent: "triage",
			Scope:  domain.Scope{Company: "acme", Area: "cx"},
			Reason: "estorno stopped holding, with nothing published since",
		})
	if err != nil {
		t.Fatalf("post: %v", err)
	}

	said := body.Blocks[0].Text.Text
	if !strings.Contains(said, "nothing published") || !strings.Contains(said, "estorno") {
		t.Errorf("summary = %q, want it to say nothing was published, and which case", said)
	}
}
