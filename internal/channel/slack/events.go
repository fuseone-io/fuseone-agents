package slack

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

/*
What the Events API delivers, read as the two things it can be.

A URL verification handshake, which is answered and nothing else, and an event
wrapper carrying the message somebody typed. They arrive on the same path with
the same signature, and telling them apart is the first thing the door does.

Only a mention is read. NT-005 §1: a trigger is a deliberate ask — a mention, a
command, a reply to a thread — never every message in the channel. The
listening version sounds more capable and is worse in three directions at once:
cost, because ambient conversation becomes tokens; privacy, because people's
conversations reach a model nobody asked to involve; and the record, which
fills with runs nobody started. The mention is the consent, and it is free.
*/

// ErrNotAnAsk means the delivery was well-formed and is not something to act
// on: a message this platform does not read, a bot talking to itself, an event
// type nobody subscribed to on purpose.
var ErrNotAnAsk = errors.New("slack: nothing here is an ask")

// Delivery is one thing that arrived on the events path.
type Delivery struct {
	// Challenge is set when Slack is verifying the URL. Answered verbatim and
	// nothing else happens.
	Challenge string
	// EventID is Slack's own identifier for this delivery, repeated on every
	// retry of it. It is what makes a redelivery cost nothing.
	EventID string

	Conversation string
	User         string
	Text         string
	// Thread is where a reply belongs: the parent when the ask came inside a
	// thread, and the message itself when it started one.
	Thread string
}

type envelope struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	EventID   string `json:"event_id"`
	Event     struct {
		Type     string `json:"type"`
		Subtype  string `json:"subtype"`
		Channel  string `json:"channel"`
		User     string `json:"user"`
		BotID    string `json:"bot_id"`
		Text     string `json:"text"`
		TS       string `json:"ts"`
		ThreadTS string `json:"thread_ts"`
	} `json:"event"`
}

// ReadDelivery reads what arrived, or says it is not an ask.
func ReadDelivery(body []byte) (Delivery, error) {
	var e envelope
	if err := json.Unmarshal(body, &e); err != nil {
		return Delivery{}, fmt.Errorf("slack: unreadable delivery: %w", err)
	}

	if e.Type == "url_verification" {
		// Answered after the signature was checked, never before: a handshake
		// is the one request where answering an unverified caller would prove
		// this endpoint exists to anybody who guesses the path.
		return Delivery{Challenge: e.Challenge}, nil
	}
	if e.Type != "event_callback" || e.EventID == "" {
		return Delivery{}, fmt.Errorf("%w: %q", ErrNotAnAsk, e.Type)
	}

	/*
		A mention and nothing else.

		`app_mention` is the event Slack sends when somebody addresses the bot,
		and subscribing to `message.channels` instead would deliver every word
		typed in every channel the bot is in. That is the difference between a
		platform people invite and one they discover reading their
		conversations.
	*/
	if e.Event.Type != "app_mention" {
		return Delivery{}, fmt.Errorf("%w: %q", ErrNotAnAsk, e.Event.Type)
	}

	// A bot's own message, or one with a subtype — an edit, a join, a file
	// share. Neither is somebody asking, and reading a bot's mention is how a
	// pair of bots discover each other and never stop.
	if e.Event.BotID != "" || e.Event.Subtype != "" || strings.TrimSpace(e.Event.User) == "" {
		return Delivery{}, fmt.Errorf("%w: not a person speaking", ErrNotAnAsk)
	}

	/*
		A signed payload missing the parts an ask is made of.

		Slack sends these, so this is not defensiveness about Slack: it is
		about everything else that can put a signed body on this path — a
		proxy that rewrote it, a replay of an older API shape, a test fixture
		somebody copied. Read leniently, a message with no channel becomes an
		arrival filed under an empty conversation, which resolves to no scope
		and refuses later, further from the cause.
	*/
	if e.Event.Channel == "" || e.Event.TS == "" {
		return Delivery{}, fmt.Errorf("%w: a mention with no conversation or no timestamp", ErrNotAnAsk)
	}

	thread := e.Event.ThreadTS
	if thread == "" {
		thread = e.Event.TS
	}
	return Delivery{
		EventID:      e.EventID,
		Conversation: e.Event.Channel,
		User:         e.Event.User,
		Text:         e.Event.Text,
		Thread:       thread,
	}, nil
}
