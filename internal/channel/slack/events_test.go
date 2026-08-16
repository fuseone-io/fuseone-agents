package slack_test

import (
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/channel/slack"
)

/*
What arrives on the events path, read as the two things it can be.

Only a mention is read. A trigger is a deliberate ask, never every message in
the channel: the listening version costs tokens on ambient conversation, puts
people's talk in front of a model nobody asked to involve, and fills the record
with runs nobody started. The mention is the consent, and it is free.
*/

func TestReadDelivery_aMention_isAnAsk(t *testing.T) {
	t.Parallel()

	got, err := slack.ReadDelivery([]byte(`{
	  "type":"event_callback","event_id":"Ev123",
	  "event":{"type":"app_mention","channel":"C07-ops","user":"U9",
	           "text":"<@U07BOT> triagem esse chamado","ts":"1786.1"}
	}`))
	if err != nil {
		t.Fatalf("ReadDelivery: %v", err)
	}
	if got.EventID != "Ev123" || got.Conversation != "C07-ops" || got.User != "U9" {
		t.Errorf("got %+v, want the ask", got)
	}
	// A reply belongs where the ask was made. An ask that started a thread is
	// its own parent.
	if got.Thread != "1786.1" {
		t.Errorf("thread = %q, want the message that started it", got.Thread)
	}
}

func TestReadDelivery_insideAThread_repliesToTheParent(t *testing.T) {
	t.Parallel()

	got, err := slack.ReadDelivery([]byte(`{
	  "type":"event_callback","event_id":"Ev124",
	  "event":{"type":"app_mention","channel":"C07-ops","user":"U9",
	           "text":"<@U07BOT> triagem","ts":"1786.9","thread_ts":"1786.1"}
	}`))
	if err != nil {
		t.Fatalf("ReadDelivery: %v", err)
	}
	if got.Thread != "1786.1" {
		t.Errorf("thread = %q, want the parent", got.Thread)
	}
}

/*
An ordinary message is not an ask, even in a channel the bot is in.

Subscribing to every message would deliver every word typed where the bot sits.
That is the difference between a platform people invite and one they discover
reading their conversations.
*/
func TestReadDelivery_anOrdinaryMessage_isNotAnAsk(t *testing.T) {
	t.Parallel()

	_, err := slack.ReadDelivery([]byte(`{
	  "type":"event_callback","event_id":"Ev125",
	  "event":{"type":"message","channel":"C07-ops","user":"U9","text":"bom dia","ts":"1786.2"}
	}`))
	if !errors.Is(err, slack.ErrNotAnAsk) {
		t.Errorf("err = %v, want ErrNotAnAsk", err)
	}
}

// A bot's own mention is how two bots discover each other and never stop.
func TestReadDelivery_aBotSpeaking_isNotAnAsk(t *testing.T) {
	t.Parallel()

	_, err := slack.ReadDelivery([]byte(`{
	  "type":"event_callback","event_id":"Ev126",
	  "event":{"type":"app_mention","channel":"C07-ops","bot_id":"B1",
	           "text":"<@U07BOT> triagem","ts":"1786.3"}
	}`))
	if !errors.Is(err, slack.ErrNotAnAsk) {
		t.Errorf("err = %v, want ErrNotAnAsk", err)
	}
}

// An edit is not somebody asking again. It arrives as a message with a
// subtype, and reading it as an ask would run an agent because a typo was
// corrected.
func TestReadDelivery_anEditedMessage_isNotAnAsk(t *testing.T) {
	t.Parallel()

	_, err := slack.ReadDelivery([]byte(`{
	  "type":"event_callback","event_id":"Ev127",
	  "event":{"type":"app_mention","subtype":"message_changed","channel":"C07-ops",
	           "user":"U9","text":"<@U07BOT> triagem","ts":"1786.4"}
	}`))
	if !errors.Is(err, slack.ErrNotAnAsk) {
		t.Errorf("err = %v, want ErrNotAnAsk", err)
	}
}

func TestReadDelivery_theUrlHandshake_comesBackToBeAnswered(t *testing.T) {
	t.Parallel()

	got, err := slack.ReadDelivery([]byte(`{"type":"url_verification","challenge":"abc123"}`))
	if err != nil {
		t.Fatalf("ReadDelivery: %v", err)
	}
	if got.Challenge != "abc123" {
		t.Errorf("challenge = %q, want it echoed", got.Challenge)
	}
}
