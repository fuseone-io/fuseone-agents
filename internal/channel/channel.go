/*
Package channel carries what a run has to say to the people waiting on it.

The first of the two families in NT-005: an internal channel, where whoever
reads it is a person this installation knows. This package holds only the
outbound half — a run reports, and nothing a conversation says can start
anything. Inbound is a separate surface with a separate threat model and it
does not belong behind the same door.

A conversation carries a scope, and that is governance rather than routing. A
channel that received another area's runs would be a way around every read
check on this platform, arriving as a notification (PRD NF-06, AU-05).
*/
package channel

import (
	"context"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// Event is the thing that happened to a run that somebody might want to know
// about. Deliberately few: a channel that hears everything is a channel people
// mute, and a muted channel is worse than none because the approval that
// mattered is in it.
type Event string

const (
	// EventParked is a run waiting on a person. The reason this stage exists.
	EventParked Event = "parked"
	// EventFailed is a run that stopped and will not continue by itself.
	EventFailed Event = "failed"
	// EventFinished is a run that ended well. Off by default.
	EventFinished Event = "finished"
)

// Report is a run, at the moment something happened to it.
type Report struct {
	RunID   domain.RunID
	AgentID domain.AgentID
	Scope   domain.Scope
	Event   Event
	At      time.Time

	// Reason is why a run parked or failed, as a code rather than a sentence:
	// the words belong to whatever renders them, in whichever language the
	// reader has.
	Reason string
	// Tool is the action a parked run is waiting for permission to take.
	Tool string
}

// Conversation is one place inside a channel, and the scope it speaks for.
type Conversation struct {
	// Channel names the configured connection, not the vendor.
	Channel string
	// ID is the conversation as the channel knows it: a Slack channel id, a
	// Teams conversation id.
	ID string
	// Label is what a person calls it, for the console and for logs.
	Label string
	// Wants is which events reach it. Empty means the defaults.
	Wants []Event
}

// wants answers whether an event belongs here.
func (c Conversation) wants(e Event) bool {
	list := c.Wants
	if len(list) == 0 {
		list = []Event{EventParked, EventFailed}
	}
	for _, want := range list {
		if want == e {
			return true
		}
	}
	return false
}

// Message is what gets posted, in parts rather than as a rendered string.
//
// The driver decides how a channel shows a heading, a set of facts and a link,
// because Slack blocks, an Adaptive Card and a plain SMS are three different
// answers to that and none of them is a format the caller should know.
type Message struct {
	Event  Event
	RunID  domain.RunID
	Agent  domain.AgentID
	Scope  domain.Scope
	Reason string
	Tool   string
	// Link is where somebody goes to act on it.
	Link string
}

// Delivery records that a message left. One per run, event and conversation.
type Delivery struct {
	RunID        domain.RunID
	Event        Event
	Conversation string
	// Ref is what the channel called the message, so a later stage can reply
	// in the same thread.
	Ref      string
	PostedAt time.Time
}

// Reports lists what has happened and not yet been said, declared here by the
// consumer.
type Reports interface {
	Unreported(ctx context.Context, since time.Time, limit int) ([]Report, error)
}

// Conversations answers which places speak for a scope.
type Conversations interface {
	For(ctx context.Context, scope domain.Scope) ([]Conversation, error)
}

// Poster is a channel driver: the one thing in this package that touches the
// outside.
type Poster interface {
	Post(ctx context.Context, c Conversation, m Message) (ref string, err error)
}

// Deliveries is what has already been said.
type Deliveries interface {
	Record(ctx context.Context, d Delivery) error
	Delivered(ctx context.Context, run domain.RunID, e Event, conversation string) (bool, error)
}

// Available is a place a connection could be pointed at, as a person would
// recognise it.
//
// The name is offered and the identifier is stored: a conversation can be
// renamed and its id cannot, so keeping what the operator recognised would
// break delivery on the day somebody tidied the workspace up.
type Available struct {
	ID      string
	Name    string
	Private bool
}

// Listers answer what a connection can be pointed at. Declared here because
// not every channel can be asked — a driver that cannot list says so, and the
// console falls back to letting somebody type an identifier.
type Listers interface {
	Conversations(ctx context.Context, channel string) ([]Available, error)
}
