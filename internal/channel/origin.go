package channel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

/*
Which scope a conversation speaks for.

The outbound half asks the opposite question — which conversations hear about a
scope — and inbound cannot be built from it. A run reports to every conversation
whose scope contains its own, and that containment is right for hearing and
wrong for asking: a company-wide channel that hears about every area is a
reasonable thing to configure, and one that can *start* an agent in every area
is a different grant entirely. Visibility and action are not symmetric, and
reading one map in both directions would make them so by accident.

So this answers exactly, and an agent is startable from a conversation when
their scopes are the same one. A company-wide conversation starts agents whose
scope is the company; an area's agents need their area's conversation, which is
the separation §4 of NT-005 exists to keep: the same person asking the same
thing in two channels gets two different sets of permitted tools.
*/

// ErrNoConversation means nothing here is configured to speak for anybody.
//
// Answered the same way as a conversation the platform has never heard of. A
// caller learning which channels this installation listens in is a caller
// mapping it.
var ErrNoConversation = errors.New("channel: no conversation by that id")

/*
ErrAmbiguousConversation means the same conversation speaks for two scopes.

Refused rather than resolved. §4 says a conversation carries *a* scope, and
taking the first row would make which one depend on the order a query happened
to return — so the same message would be governed differently on different
days, and nobody could answer "who could have asked for this".

Writing it is refused too ([admin.Channels.PutConversation]), so this is the
second of two locks. The screen stops the configuration from being made and
this stops it being trusted, because a row can also arrive by restore, by
migration, or from a version of the screen that did not check.
*/
var ErrAmbiguousConversation = errors.New("channel: that conversation speaks for more than one scope")

/*
ErrAnnouncesOnly means this conversation was configured to hear and not to ask.

Two rows reach it. One says so in its mode — a room somebody added the bot to
for visibility. The other speaks for the whole installation, which contains
every company: a room that hears about every company is a reasonable thing to
configure, and one that can start an agent in every company is a different
grant entirely.

Refused on read as well as on write. A row arrives by restore, by migration, or
from a version of the screen that did not check — and for the second kind the
safety is otherwise borrowed entirely from another package: no agent can be
published at the installation, and the catalogue compares the company for
equality rather than containment, so the startable list comes back empty. Both
are true today and neither says why. The day the second is taught to read the
sentinel as "everything", this room becomes a start button for every agent
here, and nothing in this package would have noticed.
*/
var ErrAnnouncesOnly = errors.New("channel: that conversation announces and starts nothing")

/*
Resolve answers what this conversation was configured to be.

Keyed by the connection as well as the conversation. An id means nothing on its
own: two workspaces are two namespaces, and an id naming a channel in one may
name another somewhere else — so a single argument would let a message in one
installation's Slack resolve to a scope configured for a different Teams.

Everything the consumer needs comes back from one read. Scope, agent and mode
answered by separate calls could each see a different version of the
configuration, and an ask governed by one row's scope and another row's agent
is a combination nobody configured.
*/
func (c *Configured) Resolve(ctx context.Context, channel, id string) (Mapped, error) {
	stored, err := c.store.List(ctx, KindConversation)
	if err != nil {
		return Mapped{}, fmt.Errorf("channel: list conversations: %w", err)
	}

	var found []Mapped
	for _, s := range stored {
		if s.Name != id || !s.Enabled {
			continue
		}
		// The row is read for the connection it belongs to. Its contents do
		// not decide the scope: the scope is the row's own, which is
		// administrative and not something a conversation's configuration can
		// widen.
		var v conversationValue
		if err := json.Unmarshal(s.Value, &v); err != nil {
			continue
		}
		if v.Channel != channel {
			continue
		}
		// The stored value, not the normalised one. ConversationMode answers
		// anything it does not recognise with "mentions", which is the right
		// answer for a screen and the opposite of the right answer for
		// deciding who may start work: a row written by a newer version and
		// restored here would come back as a conversation anybody can start
		// runs from by typing in it.
		found = append(found, Mapped{Scope: s.Scope, Agent: v.Agent, Mode: v.Mode})
	}

	switch len(found) {
	case 0:
		return Mapped{}, fmt.Errorf("%w: %s/%s", ErrNoConversation, channel, id)
	case 1:
		// After the count and not inside the search. Refusing while looking
		// would answer "this one announces only" and hide the fact that two
		// rows exist, sending an operator to the wrong one.
		if found[0].Scope.IsInstallation() || !startsSomething(found[0].Mode) {
			return Mapped{}, fmt.Errorf("%w: %s/%s", ErrAnnouncesOnly, channel, id)
		}
		return found[0], nil
	default:
		return Mapped{}, fmt.Errorf("%w: %s/%s", ErrAmbiguousConversation, channel, id)
	}
}
