package slack

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/channel"
)

/*
What the Events API delivers, read as the two things it can be.

A URL verification handshake, which is answered and nothing else, and an event
wrapper carrying the message somebody typed. They arrive on the same path with
the same signature, and telling them apart is the first thing the door does.

Only a mention is read by default. A watched message is a separate
configuration on a conversation: the source is filtered first, and the
authority comes from the rule, never from whoever wrote the Slack line.
*/

// ErrNotAnAsk means the delivery was well-formed and is not something to act
// on: a message this platform does not read, a bot talking to itself, an event
// type nobody subscribed to on purpose.
var ErrNotAnAsk = errors.New("slack: nothing here is an ask")

/*
ErrMalformedAsk means a mention arrived without the parts an ask is made of.

Separate from ErrNotAnAsk because the two deserve different answers. A message
that is legitimately not for us — an ordinary line, a bot, an edit — is
acknowledged and forgotten: there is nothing to fix and a retry would deliver
it forever. A mention with no conversation is something we could not read, and
answering that with a quiet 200 makes it invisible to everybody, including the
proxy or the fixture that produced it.

The same distinction the whole platform keeps: a state is answered, a failure
is reported.
*/
var ErrMalformedAsk = errors.New("slack: a mention with nothing to act on")

// Delivery is one thing that arrived on the events path.
type Delivery struct {
	Kind string
	// Challenge is set when Slack is verifying the URL. Answered verbatim and
	// nothing else happens.
	Challenge string
	// EventID is Slack's own identifier for this delivery, repeated on every
	// retry of it. It is what makes a redelivery cost nothing, and it is not
	// the message: two retries of one message share it, and it appears in no
	// thread.
	EventID string
	// Message is what the channel calls the message somebody typed. This is
	// what a thread is keyed by and what an origin points at.
	Message string

	Conversation string
	User         string
	Source       channel.Source
	Text         string
	// Thread is where a reply belongs: the parent when the ask came inside a
	// thread, and the message itself when it started one.
	Thread string
}

const (
	DeliveryMention = "mention"
	DeliveryMessage = "message"
)

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
		AppID    string `json:"app_id"`
		Text     string `json:"text"`
		TS       string `json:"ts"`
		ThreadTS string `json:"thread_ts"`
	} `json:"event"`
}

// ReadDelivery reads what arrived, or says it is not an ask.
func ReadDelivery(body []byte) (Delivery, error) {
	return readDelivery(body, false)
}

// ReadAnyDelivery reads mentions and ordinary messages. The caller is
// responsible for checking the ordinary message against a configured watch
// rule before writing it to the inbox.
func ReadAnyDelivery(body []byte) (Delivery, error) {
	return readDelivery(body, true)
}

func readDelivery(body []byte, allowMessages bool) (Delivery, error) {
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

	if e.Event.Type == "message" && allowMessages {
		return ordinaryMessage(e)
	}
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
		return Delivery{}, fmt.Errorf("%w: no conversation or no timestamp", ErrMalformedAsk)
	}

	thread := e.Event.ThreadTS
	if thread == "" {
		thread = e.Event.TS
	}
	return Delivery{
		Kind:         DeliveryMention,
		EventID:      e.EventID,
		Message:      e.Event.TS,
		Conversation: e.Event.Channel,
		User:         e.Event.User,
		Source:       sourceOf(e),
		Text:         e.Event.Text,
		Thread:       thread,
	}, nil
}

func ordinaryMessage(e envelope) (Delivery, error) {
	// Edits, deletes, joins and file shares are not a new alert. Bot messages
	// are the ordinary form an alerting system posts, so they are allowed
	// through to the watch-rule filter.
	if e.Event.Subtype != "" && e.Event.Subtype != "bot_message" {
		return Delivery{}, fmt.Errorf("%w: not a plain message", ErrNotAnAsk)
	}
	source := sourceOf(e)
	if source.Key() == "" {
		return Delivery{}, fmt.Errorf("%w: no source", ErrNotAnAsk)
	}
	if e.Event.Channel == "" || e.Event.TS == "" {
		return Delivery{}, fmt.Errorf("%w: no conversation or no timestamp", ErrMalformedAsk)
	}
	thread := e.Event.ThreadTS
	if thread == "" {
		thread = e.Event.TS
	}
	return Delivery{
		Kind:         DeliveryMessage,
		EventID:      e.EventID,
		Message:      e.Event.TS,
		Conversation: e.Event.Channel,
		User:         e.Event.User,
		Source:       source,
		Text:         e.Event.Text,
		Thread:       thread,
	}, nil
}

func sourceOf(e envelope) channel.Source {
	return channel.Source{User: e.Event.User, Bot: e.Event.BotID, App: e.Event.AppID}
}
