package channel

import (
	"context"
	"unicode/utf8"

	"github.com/fuseone/agents/internal/ticket"
)

// Why a message in a ticket room started nothing. Said at debug, because a
// room where most messages are conversation would otherwise narrate itself —
// and because the question is only ever asked while somebody is looking for a
// mark that did not land.
const (
	notMarkedShape   = "not a plain message"
	notMarkedSource  = "the source may not mark threads"
	notMarkedPattern = "no pattern matched"
	notMarkedRoom    = "the room does not open tickets from marks"
	notClosingEmoji  = "the emoji does not close tickets here"
)

/*
mark admits a thread nobody could have opened from its root.

Some help channels are written by a form: the person fills it in, a bot posts
it, and which team owns the request is decided afterwards, in the thread. The
root is therefore neither a person's message nor a match for anything, and the
first message that says what this is arrives as a reply.

So the reply admits the thread, and the ticket keeps the root's identity — the
request is what the root says, and every later reply belongs to the same
ticket. The mark is routing and never authority: who may decide is still the
addressing source and the scope's approvers, and whose root may be admitted at
all is the rule's RootFrom, checked where the root can be read.
*/
func (r *TicketRoutes) mark(
	ctx context.Context, candidate TicketCandidate,
) (TicketIntent, bool, error) {
	if candidate.Kind != "message" || len(candidate.Text) > MaxTicketMatchBytes ||
		!utf8.ValidString(candidate.Text) {
		return TicketIntent{}, false, nil
	}
	admitted, ok, err := r.admission(ctx, candidate)
	if err != nil || !ok {
		return TicketIntent{}, false, err
	}
	rule := admitted.compiled.rule
	switch {
	case rule.OpenFrom != TicketOpenMarkedThreads:
		return r.ignored(candidate, notMarkedRoom)
	case !marksTickets(candidate.Source, rule):
		return r.ignored(candidate, notMarkedSource)
	case !matchesTicket(admitted.compiled.patterns, candidate.Text):
		return r.ignored(candidate, notMarkedPattern)
	}
	key, err := ticket.Key(candidate.Connection, candidate.Conversation, candidate.Thread)
	if err != nil {
		return TicketIntent{}, false, err
	}
	return TicketIntent{
		Key: key, Root: true, Marked: true, Scope: admitted.scope,
		Agent: admitted.value.Agent, RunAs: admitted.value.RunAs,
		AddressedBy: rule.AddressFrom, RootFrom: rule.RootFrom, ReviewIn: rule.ReviewIn,
	}, true, nil
}

/*
marksTickets answers who may say that a thread is a ticket.

The one configured triage source, or a person in the channel. A person is a
message Slack attributes to an account and not to a bot: an app id alone does
not make it one, because Slack puts the app's id on everything a person sends
through an integration — a client, a workflow, an assistant writing with their
token. Reading that as "a bot wrote it" ignores the people this exists for.

A bot id is different: that is the app speaking as itself, and any bot but the
configured one is ignored, so an integration echoing the mark cannot open work.
*/
func marksTickets(source Source, rule TicketRule) bool {
	if source.MatchesKey(rule.AddressFrom) {
		return true
	}
	return source.Person()
}

// ignored says why a thread was left alone. The message's text is never part
// of it: this runs for every message in the room, and what people write in a
// support channel is not ours to copy into a log.
func (r *TicketRoutes) ignored(candidate TicketCandidate, why string) (TicketIntent, bool, error) {
	r.logger().Debug("a message in a ticket room opened nothing",
		"channel", candidate.Connection, "conversation", candidate.Conversation,
		"message", candidate.Message, "source", candidate.Source.Key(), "why", why)
	return TicketIntent{}, false, nil
}
