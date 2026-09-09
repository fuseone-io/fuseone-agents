package slack_test

import (
	"errors"
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
	server, _, body := recordingCall(t, `{"ok":true,"ts":"1.1"}`)
	return server, body
}

// recordingCall answers whatever it is told to and remembers both halves of
// the request. Which endpoint was called is half of what these tests assert:
// posting and editing carry nearly the same body and are entirely different
// acts, and a mistake between them is invisible in the body alone.
func recordingCall(t *testing.T, answer string) (*httptest.Server, *string, *string) {
	t.Helper()
	path, body := "", ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		path, body = r.URL.Path, string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(server.Close)
	return server, &path, &body
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

/*
Rewriting a card is a different call from posting one.

The two carry nearly the same body, so a mistake between them is invisible in
the body alone — and it is not a small mistake: posting instead of editing
leaves the old card offering an answer and adds a second one saying the
question is settled, in every conversation and every private message.

The place is the other half. chat.postMessage takes a person and opens the
conversation itself; chat.update does not, so it takes the one Slack named.
*/
func TestEdit_rewritesTheMessageWhereSlackPutIt(t *testing.T) {
	t.Parallel()
	server, path, sent := recordingCall(t, `{"ok":true,"ts":"1.1"}`)

	answered := parked()
	answered.Outcome = channel.OutcomeApproved
	answered.DecidedBy = "usr_ana"

	err := slack.New("xoxb-test").WithEndpointBase(server.URL).Decidable().
		Edit(t.Context(), channel.Placement{
			Channel: "acme-slack", Conversation: "D-ana", Ref: "1786.7",
		}, answered)
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}

	if *path != "/chat.update" {
		t.Errorf("called %q, want chat.update", *path)
	}
	for _, want := range []string{`"channel":"D-ana"`, `"ts":"1786.7"`} {
		if !strings.Contains(*sent, want) {
			t.Errorf("the edit does not carry %s:\n%s", want, *sent)
		}
	}
	// Twice, and twice is right: the fallback text and the heading, which is
	// what every message here carries. A third is the outcome repeated as a
	// note underneath, telling the reader the same thing again while the facts
	// between the two go unread.
	if n := strings.Count(*sent, "Approved by usr_ana"); n != 2 {
		t.Errorf("the card says what happened %d times, want the fallback and the heading:\n%s",
			n, *sent)
	}
	if strings.Contains(*sent, `"type":"actions"`) {
		t.Error("the rewritten card still offers an answer")
	}
}

/*
Slack's words for a person who cannot be messaged.

They have to be recognised as final. Read as an ordinary failure, a run whose
approver left the workspace is held open and every recipient is attempted again
every thirty seconds until the window closes — and the card that did arrive is
never marked as said.
*/
func TestPost_refusalsAboutTheRecipient_areReportedAsPermanent(t *testing.T) {
	t.Parallel()

	for _, reason := range []string{
		"cannot_dm_bot", "user_not_found", "users_not_found",
		"message_not_found", "cant_update_message",
	} {
		server, _, _ := recordingCall(t, `{"ok":false,"error":"`+reason+`"}`)

		_, err := slack.New("xoxb-test").WithEndpointBase(server.URL).
			Post(t.Context(), channel.Conversation{ID: "U-gone"}, parked())
		var refused *channel.Error
		if !errors.As(err, &refused) {
			t.Fatalf("%s: err = %v, want a channel error", reason, err)
		}
		if refused.Code != channel.CodeConversationUnavailable {
			t.Errorf("%s: code = %q, want the conversation reported unavailable",
				reason, refused.Code)
		}
	}
}
