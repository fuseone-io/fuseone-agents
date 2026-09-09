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
		AwaitingDecision: true,
		Link:             "https://agents.example.com/runs/run-1",
	}
}

/*
A run that stopped without asking anybody gets no buttons.

A budget park and a retry that stopped helping carry a step now, because an
announcement is keyed by the step a run stopped on. Read as "a park with a step
is a pending approval" they draw Approve and Refuse, whose only possible answer
is a conflict — and the heading says a run is waiting for a decision nobody can
give.
*/
func TestPost_aStopWithNothingToDecide_offersNoAnswer(t *testing.T) {
	t.Parallel()

	stopped := parked()
	stopped.AwaitingDecision = false
	stopped.Reason = "over the budget"
	server, sent := recording(t)
	if _, err := slack.New("xoxb-test").WithEndpointBase(server.URL).Decidable().
		Post(t.Context(), channel.Conversation{ID: "C07"}, stopped); err != nil {
		t.Fatalf("post: %v", err)
	}

	for _, unwanted := range []string{"approve:run-1:12", "refuse:run-1:12", "waiting"} {
		if strings.Contains(*sent, unwanted) {
			t.Errorf("the message carries %q for a stop nobody can answer", unwanted)
		}
	}
	if !strings.Contains(*sent, "over the budget") {
		t.Errorf("the message does not say why it stopped:\n%s", *sent)
	}
}

// A card whose question has an answer says so in its heading and in the
// fallback text, which is what a phone notification reads aloud. Left alone,
// both went on saying a run is waiting with the answer underneath.
func TestEdit_ananswered_cardDoesNotStillSayItIsWaiting(t *testing.T) {
	t.Parallel()

	answered := parked()
	answered.Outcome = channel.OutcomeApproved
	answered.DecidedBy = "usr_ana"
	server, sent := recording(t)
	if _, err := slack.New("xoxb-test").WithEndpointBase(server.URL).Decidable().
		Post(t.Context(), channel.Conversation{ID: "C07"}, answered); err != nil {
		t.Fatalf("post: %v", err)
	}

	if strings.Contains(*sent, "waiting") {
		t.Errorf("an answered card still says it is waiting:\n%s", *sent)
	}
	if !strings.Contains(*sent, "Approved by usr_ana") {
		t.Errorf("the card does not say who answered:\n%s", *sent)
	}
}
