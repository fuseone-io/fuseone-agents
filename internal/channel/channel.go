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
	// EventGateRefusal is the first time a particular Gate block appears in
	// a scope. It rides with failed notifications for now: the same people
	// who need to know a run stopped need to know a new shape of stop exists.
	EventGateRefusal Event = "gate_refusal"
	// EventFinished is a run that ended well. Off by default.
	EventFinished Event = "finished"
	// EventDrifted is an agent that stopped holding its corrections with
	// nothing published since. Not a run: it is the one notice here that is
	// about the world moving rather than about work in progress, and it is
	// the reason anybody keeps a corpus in an installation nobody watches.
	EventDrifted Event = "drifted"
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
	// AwaitingDecision is whether this stop is a question somebody can answer.
	// Read from the run's phase, never from the sequence: a run parked by a
	// budget carries one and has nothing to decide.
	AwaitingDecision bool
	// AtSeq is the step the run is waiting on, which a decision has to name.
	// A button carrying only the run would answer whatever the run happens to
	// be waiting on when it is pressed, and a message keeps its buttons for
	// ever.
	AtSeq int64
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
	// Agent names the agent this conversation starts — from a watched message,
	// and from a mention that does not name one. Empty means a conversation
	// open to whatever its scope publishes.
	Agent domain.AgentID
	// Wants is which events reach it. Empty means the defaults.
	Wants []Event
	// DirectApprovals says an approval announced here also goes privately to
	// the people who may decide it. Outbound and orthogonal to Mode: which
	// messages may start a run here is a different question from who is told
	// when one stops.
	DirectApprovals bool
}

// wants answers whether an event belongs here.
func (c Conversation) wants(e Event) bool {
	list := c.Wants
	if len(list) == 0 {
		// Drift is in the defaults, unlike everything else that had to be
		// asked for. It fires rarely, and it is the one notice nobody would
		// think to opt into: an agent that quietly stopped working is
		// precisely what somebody does not know to go looking for.
		list = []Event{EventParked, EventFailed, EventDrifted}
	}
	for _, want := range list {
		if want == e || want == failedFamily(e) {
			return true
		}
	}
	return false
}

/*
Wants answers whether a list of chosen events includes this one.

Exported because two places need the same answer and one of them is not this
package. A conversation is told about parked runs or it is not, and the
administration decides what to store on the strength of that — a second
statement of the rule in admin would be a second definition of what an empty
list means, and the two would drift the first time the defaults changed.
*/
func Wants(list []Event, e Event) bool {
	return Conversation{Wants: list}.wants(e)
}

func (c Conversation) reportsAgent(agent domain.AgentID) bool {
	return c.Agent == "" || agent == "" || c.Agent == agent
}

func failedFamily(e Event) Event {
	if e == EventGateRefusal {
		return EventFailed
	}
	return ""
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
	AtSeq  int64
	// Link is where somebody goes to act on it.
	Link string
	// AwaitingDecision is whether the run is stopped on a question somebody
	// can answer. A run stops for other reasons — a budget, retries that
	// stopped helping — and those carry a sequence too now, so the sequence
	// alone cannot tell them apart. Buttons and private messages both hang on
	// this: an answer to a run that is not asking can only ever be a conflict.
	AwaitingDecision bool
	// Outcome is what happened to the approval this card asked about. Set only
	// on a replacement: a card carrying one offers no buttons, because the
	// question it asked already has an answer.
	Outcome   Outcome
	DecidedBy string
}

/*
Announcement names one message the platform owes.

Which run, about which of its steps, on which connection, in which
conversation. It is the idempotency key of the whole outbound path, written
once as a type because its parts are easy to transpose: the connection and the
conversation are both strings, and a caller that swapped them would ask whether
a message had reached a place that does not exist and be told no, for ever.
*/
type Announcement struct {
	RunID domain.RunID
	Event Event
	// AtSeq is the step this message is about.
	//
	// Zero for an event a run has once — it finished, it failed. The sequence
	// of the request for a park, because a run stops as many times as it asks:
	// an agent that wants two tools asks for two approvals, and keyed by the
	// run alone the second question was never asked of anybody.
	AtSeq int64
	// Channel names the connection the conversation belongs to. A conversation
	// id means nothing on its own: two workspaces are two namespaces, and an
	// id that names a channel in one may name another somewhere else.
	Channel      string
	Conversation string
}

// Reports lists what has happened and not yet been said, declared here by the
// consumer.
type Reports interface {
	Unreported(ctx context.Context, since time.Time, limit int) ([]Report, error)
	// Reported marks one report as said everywhere it should be said. The
	// reporter is the only component that knows what everywhere means, so it
	// is the one that writes it.
	//
	// The whole report rather than its parts, because the step it is about is
	// half the key: a run reported at the step it stopped on is still owed an
	// announcement the next time it stops.
	Reported(ctx context.Context, r Report, at time.Time) error
}

// AnnouncementTo is what this report owes one conversation.
func (r Report) AnnouncementTo(place Conversation) Announcement {
	return Announcement{
		RunID: r.RunID, Event: r.Event, AtSeq: r.AtSeq,
		Channel: place.Channel, Conversation: place.ID,
	}
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
