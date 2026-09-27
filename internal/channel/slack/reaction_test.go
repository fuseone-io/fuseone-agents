package slack_test

import (
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/channel/slack"
)

/*
A reaction is how a team says a request is done.

It is not an ask and never starts anything: what it carries is an emoji, the
message it was put on and who put it there. Everything else about it — whether
that message is a ticket, whether that emoji closes anything, whether that
person may — is decided where tickets are.
*/
func TestReadAnyDelivery_aReactionOnAMessage_isReadAsOne(t *testing.T) {
	t.Parallel()
	delivery, err := slack.ReadAnyDelivery([]byte(`{
		"type": "event_callback",
		"event_id": "Ev-react",
		"event": {
			"type": "reaction_added",
			"user": "U-manager",
			"reaction": "white_check_mark",
			"item": {"type": "message", "channel": "C-help", "ts": "171.1"}
		}
	}`))
	if err != nil {
		t.Fatalf("ReadAnyDelivery: %v", err)
	}
	if delivery.Kind != slack.DeliveryReaction || delivery.Reaction != "white_check_mark" {
		t.Fatalf("delivery = %+v, want the reaction", delivery)
	}
	if delivery.Conversation != "C-help" || delivery.Message != "171.1" ||
		delivery.Thread != "171.1" || delivery.User != "U-manager" {
		t.Fatalf("delivery = %+v, want the message it was put on", delivery)
	}
}

func TestReadAnyDelivery_aReactionOnSomethingElse_isNotAnAsk(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"a file": `{"type":"event_callback","event_id":"Ev-1","event":{"type":"reaction_added",
			"user":"U-1","reaction":"eyes","item":{"type":"file","file":"F-1"}}}`,
		"nobody reacting": `{"type":"event_callback","event_id":"Ev-2","event":{"type":"reaction_added",
			"reaction":"eyes","item":{"type":"message","channel":"C-help","ts":"171.1"}}}`,
		"no emoji": `{"type":"event_callback","event_id":"Ev-3","event":{"type":"reaction_added",
			"user":"U-1","item":{"type":"message","channel":"C-help","ts":"171.1"}}}`,
		"a reaction taken away": `{"type":"event_callback","event_id":"Ev-4","event":{"type":"reaction_removed",
			"user":"U-1","reaction":"eyes","item":{"type":"message","channel":"C-help","ts":"171.1"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := slack.ReadAnyDelivery([]byte(body)); !errors.Is(err, slack.ErrNotAnAsk) {
				t.Fatalf("ReadAnyDelivery = %v, want ErrNotAnAsk", err)
			}
		})
	}
}
