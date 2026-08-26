package channel

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
Notice posts one message to the conversations that speak for a scope.

Separate from Reporter because what it carries is not a run. Drift is a fact
about an agent over time — nobody changed anything and it stopped working —
and there is no run to link to, no step to decide, and nothing to record a
delivery against. Sending it through a path built around a run identifier
would mean inventing one.
*/
type Notice struct {
	conversations Conversations
	poster        Poster
}

func NewNotice(conversations Conversations, poster Poster) *Notice {
	return &Notice{conversations: conversations, poster: poster}
}

// Announce says one thing to everywhere that speaks for this scope and wants
// to hear it.
//
// A conversation that refuses is named and the rest are still told: the
// ordinary cause is a bot removed from one channel, and letting that silence
// the others would turn one misconfiguration into no notice at all.
func (n *Notice) Announce(ctx context.Context, scope domain.Scope, m Message) error {
	_, err := n.AnnounceCount(ctx, scope, m)
	return err
}

// AnnounceCount is Announce plus how many messages left.
func (n *Notice) AnnounceCount(ctx context.Context, scope domain.Scope, m Message) (int, error) {
	places, err := n.conversations.For(ctx, scope)
	if err != nil {
		return 0, fmt.Errorf("channel: conversations for %s: %w", scope, err)
	}

	sent := 0
	failures := []error{}
	for _, place := range places {
		if !place.wants(m.Event) || !place.reportsAgent(m.Agent) {
			continue
		}
		if _, err := n.poster.Post(ctx, place, m); err != nil {
			failures = append(failures, fmt.Errorf("channel: post to %s: %w", place.Label, err))
			continue
		}
		sent++
	}
	return sent, errors.Join(failures...)
}
