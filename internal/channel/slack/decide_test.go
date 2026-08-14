package slack_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/channel/slack"
)

/*
Buttons only where an answer could arrive.

A channel with no signing secret cannot verify what comes back, so a button on
one of its messages would promise an inbound surface that is switched off. That
is the worst kind of interface and it would be on the message that matters most
— the one somebody is waiting on.
*/
func TestPost_channelCannotVerifyAnAnswer_sendsNoButtons(t *testing.T) {
	t.Parallel()
	server, sent := recording(t)

	_, err := slack.New("xoxb-test").WithEndpointBase(server.URL).
		Post(t.Context(), channel.Conversation{ID: "C07"}, parked())
	if err != nil {
		t.Fatalf("post: %v", err)
	}

	if strings.Contains(*sent, `"type":"actions"`) {
		t.Error("a channel that cannot check an answer offered a button")
	}
	if !strings.Contains(*sent, "Open the run") {
		t.Error("without a button it has to at least link to the run")
	}
}

func TestPost_channelCanVerify_offersBothAnswers(t *testing.T) {
	t.Parallel()
	server, sent := recording(t)

	_, err := slack.New("xoxb-test").WithEndpointBase(server.URL).Decidable().
		Post(t.Context(), channel.Conversation{ID: "C07"}, parked())
	if err != nil {
		t.Fatalf("post: %v", err)
	}

	// Each button says what it answers and about which step. One carrying only
	// the run would answer whatever the run happens to be waiting on when it
	// is pressed, and a message keeps its buttons for ever.
	for _, want := range []string{
		slack.Decision("run-1", 12, true),
		slack.Decision("run-1", 12, false),
	} {
		if !strings.Contains(*sent, want) {
			t.Errorf("the message does not carry %q", want)
		}
	}
}

// A run that failed is not a run anybody can answer. Buttons on one would be a
// question about something already over.
func TestPost_runFailed_offersNoAnswer(t *testing.T) {
	t.Parallel()
	server, sent := recording(t)

	failed := parked()
	failed.Event = channel.EventFailed

	if _, err := slack.New("xoxb-test").WithEndpointBase(server.URL).Decidable().
		Post(t.Context(), channel.Conversation{ID: "C07"}, failed); err != nil {
		t.Fatalf("post: %v", err)
	}
	if strings.Contains(*sent, `"type":"actions"`) {
		t.Error("a run that failed was offered for decision")
	}
}

func recording(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	body := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"ts":"1.1"}`)
	}))
	t.Cleanup(server.Close)
	return server, &body
}

func parked() channel.Message {
	return channel.Message{
		Event: channel.EventParked,
		RunID: "run-1", Agent: "triage", Tool: "crm.reply", AtSeq: 12,
		Link: "https://agents.example.com/runs/run-1",
	}
}
