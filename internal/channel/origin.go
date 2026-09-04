package channel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
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
ScopeOf answers which scope an ask in this conversation belongs to.

Keyed by the connection as well as the conversation. An id means nothing on its
own: two workspaces are two namespaces, and an id naming a channel in one may
name another somewhere else — so a single argument would let a message in one
installation's Slack resolve to a scope configured for a different Teams.
*/
func (c *Configured) ScopeOf(ctx context.Context, channel, id string) (domain.Scope, error) {
	stored, err := c.store.List(ctx, KindConversation)
	if err != nil {
		return domain.Scope{}, fmt.Errorf("channel: list conversations: %w", err)
	}

	var found []domain.Scope
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
		found = append(found, s.Scope)
	}

	switch len(found) {
	case 0:
		return domain.Scope{}, fmt.Errorf("%w: %s/%s", ErrNoConversation, channel, id)
	case 1:
		return found[0], nil
	default:
		return domain.Scope{}, fmt.Errorf("%w: %s/%s", ErrAmbiguousConversation, channel, id)
	}
}

/*
AgentOf answers which agent this conversation starts.

Empty when nobody chose one, which is a configuration and not a failure: a
conversation may be open to whatever its scope publishes, and that was the only
arrangement before a conversation could name an agent.

Keyed by the connection like ScopeOf, and for the same reason. Ambiguity is not
resolved here because it is already refused there: nothing reaches this without
a scope, and a conversation speaking for two never gets one.
*/
func (c *Configured) AgentOf(ctx context.Context, channel, id string) (domain.AgentID, error) {
	stored, err := c.store.List(ctx, KindConversation)
	if err != nil {
		return "", fmt.Errorf("channel: list conversations: %w", err)
	}
	for _, s := range stored {
		if s.Name != id || !s.Enabled {
			continue
		}
		var v conversationValue
		if err := json.Unmarshal(s.Value, &v); err != nil {
			continue
		}
		if v.Channel != channel {
			continue
		}
		return v.Agent, nil
	}
	return "", nil
}
