package channel

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
Channels and conversations, as configuration rather than as tables.

Both are settings, which buys three things that would otherwise be built twice:
the credential is sealed by the vault, the change is recorded in the
administrative trail, and the value is scoped — so a conversation belongs to an
area by being configured there, not by carrying an area column somebody has to
remember to filter on.
*/

// Kinds this package stores. The connection holds the credential; the
// conversation holds no secret at all, which is why they are two.
const (
	KindChannel      settings.Kind = "channel"
	KindConversation settings.Kind = "channel_conversation"
)

// Connection is the non-secret half of a channel: which vendor, and anything
// an operator needs to recognise it. Building a driver from one lives in
// internal/channel/connect: a vendor package imports these types, so this
// package cannot import a vendor package back.
type Connection struct {
	// Kind is the vendor: slack today, teams next.
	Kind string `json:"kind"`
	// Workspace is what a person calls it. Never used to address anything.
	Workspace string `json:"workspace,omitempty"`
}

// conversationValue is a conversation as stored.
type conversationValue struct {
	Channel string  `json:"channel"`
	Label   string  `json:"label,omitempty"`
	Wants   []Event `json:"wants,omitempty"`
}

// Configured reads channels and conversations from the administration area.
type Configured struct{ store *settings.Store }

func NewConfigured(store *settings.Store) *Configured { return &Configured{store: store} }

// For answers which conversations speak for a scope.
//
// A conversation configured at company level covers every area in it, which is
// how a single #alertas serves a company that has not split its areas yet.
func (c *Configured) For(ctx context.Context, scope domain.Scope) ([]Conversation, error) {
	stored, err := c.store.List(ctx, KindConversation)
	if err != nil {
		return nil, fmt.Errorf("channel: list conversations: %w", err)
	}

	var out []Conversation
	for _, s := range stored {
		if !s.Enabled || !s.Scope.Contains(scope) {
			continue
		}
		var v conversationValue
		if err := json.Unmarshal(s.Value, &v); err != nil {
			// One malformed row must not silence every other conversation.
			continue
		}
		out = append(out, Conversation{
			Channel: v.Channel, ID: s.Name, Label: v.Label, Wants: v.Wants,
		})
	}
	return out, nil
}
