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

// ScopeOf answers which scope an ask in this conversation belongs to.
func (c *Configured) ScopeOf(ctx context.Context, id string) (domain.Scope, error) {
	stored, err := c.store.List(ctx, KindConversation)
	if err != nil {
		return domain.Scope{}, fmt.Errorf("channel: list conversations: %w", err)
	}

	for _, s := range stored {
		if s.Name != id || !s.Enabled {
			continue
		}
		// The row is read to confirm it is a conversation and not something
		// else filed under the same kind. Its contents do not decide the
		// scope: the scope is the row's own, which is administrative and not
		// something the conversation's configuration can widen.
		var v conversationValue
		if err := json.Unmarshal(s.Value, &v); err != nil {
			continue
		}
		return s.Scope, nil
	}
	return domain.Scope{}, fmt.Errorf("%w: %s", ErrNoConversation, id)
}
