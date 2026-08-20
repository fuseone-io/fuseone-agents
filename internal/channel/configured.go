package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

const (
	// DeliveryHTTP means Slack reaches this installation through the public
	// webhook path. Empty stored values read as HTTP because that was the only
	// mode before Socket Mode existed.
	DeliveryHTTP = "http"
	// DeliverySocket means the worker opens Slack Socket Mode outbound and
	// receives Events API payloads there.
	DeliverySocket = "socket"
)

const (
	// ConversationMentions means only a deliberate mention of the channel bot
	// may start an agent. Empty stored values read this way for compatibility
	// with conversations configured before watched messages existed.
	ConversationMentions = "mentions"
	// ConversationWatch means selected ordinary messages from configured
	// sources may start one configured agent under one configured principal.
	ConversationWatch = "watch"
	// ConversationBoth keeps the deliberate mention path and also watches
	// selected ordinary messages. The two paths keep their own authority:
	// mentions come from the bound person; watched messages come from RunAs.
	ConversationBoth = "both"
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
	// DeliveryMode is how asks reach this installation. Empty is HTTP for
	// compatibility with channels configured before Socket Mode existed.
	DeliveryMode string `json:"deliveryMode,omitempty"`
}

func DeliveryMode(mode string) string {
	if mode == DeliverySocket {
		return DeliverySocket
	}
	return DeliveryHTTP
}

func ConversationMode(mode string) string {
	switch mode {
	case ConversationWatch:
		return ConversationWatch
	case ConversationBoth:
		return ConversationBoth
	default:
		return ConversationMentions
	}
}

func StartsFromMentions(mode string) bool {
	return ConversationMode(mode) != ConversationWatch
}

func StartsFromWatch(mode string) bool {
	mode = ConversationMode(mode)
	return mode == ConversationWatch || mode == ConversationBoth
}

// Source is who wrote a channel event as the vendor names it.
//
// It is not authority. Authority comes from a configured RunAs principal on a
// watched conversation, or from a person binding on a mention.
type Source struct {
	User string `json:"user,omitempty"`
	Bot  string `json:"bot,omitempty"`
	App  string `json:"app,omitempty"`
}

// Key is the stable correspondent used for ceilings and the record.
func (s Source) Key() string {
	switch {
	case s.Bot != "":
		return "bot:" + s.Bot
	case s.App != "":
		return "app:" + s.App
	case s.User != "":
		return "user:" + s.User
	default:
		return ""
	}
}

func (s Source) Matches(allowed []string) bool {
	for _, one := range allowed {
		needle := strings.TrimSpace(one)
		if needle == "" {
			continue
		}
		if strings.EqualFold(needle, s.User) ||
			strings.EqualFold(needle, s.Bot) ||
			strings.EqualFold(needle, s.App) {
			return true
		}
	}
	return false
}

// conversationValue is a conversation as stored.
type conversationValue struct {
	Channel string `json:"channel"`
	Label   string `json:"label,omitempty"`
	// Mode governs inbound starts. Wants governs outbound announcements; they
	// are deliberately separate decisions.
	Mode          string         `json:"mode,omitempty"`
	Sources       []string       `json:"sources,omitempty"`
	Agent         domain.AgentID `json:"agent,omitempty"`
	RunAs         domain.UserID  `json:"runAs,omitempty"`
	ThreadContext bool           `json:"threadContext,omitempty"`
	Wants         []Event        `json:"wants,omitempty"`
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

// WatchRule is the explicit automation a watched message may start.
type WatchRule struct {
	Agent   domain.AgentID
	RunAs   domain.UserID
	Sources []string
}

// WatchFor returns the automation rule for this source in this conversation.
//
// The source is a filter, not authority. The returned RunAs is the principal a
// person configured beforehand; a Slack bot id never becomes a FuseOne user.
func (c *Configured) WatchFor(
	ctx context.Context, channelName, id string, source Source,
) (WatchRule, bool, error) {
	stored, err := c.store.List(ctx, KindConversation)
	if err != nil {
		return WatchRule{}, false, fmt.Errorf("channel: list conversations: %w", err)
	}

	for _, s := range stored {
		if s.Name != id || !s.Enabled {
			continue
		}
		var v conversationValue
		if err := json.Unmarshal(s.Value, &v); err != nil {
			continue
		}
		if v.Channel != channelName || !StartsFromWatch(v.Mode) {
			continue
		}
		if v.Agent == "" || v.RunAs == "" || !source.Matches(v.Sources) {
			return WatchRule{}, false, nil
		}
		return WatchRule{Agent: v.Agent, RunAs: v.RunAs, Sources: v.Sources}, true, nil
	}
	return WatchRule{}, false, nil
}

// IncludeThreadContext answers whether a mention-capable conversation chose to send
// earlier thread messages into the run input. It intentionally does not apply
// to watched messages: those start from the message itself, while this option
// covers a person replying to an existing alert thread with a mention.
func (c *Configured) IncludeThreadContext(
	ctx context.Context, channelName, id string,
) (bool, error) {
	stored, err := c.store.List(ctx, KindConversation)
	if err != nil {
		return false, fmt.Errorf("channel: list conversations: %w", err)
	}
	for _, s := range stored {
		if s.Name != id || !s.Enabled {
			continue
		}
		var v conversationValue
		if err := json.Unmarshal(s.Value, &v); err != nil {
			continue
		}
		if v.Channel != channelName || !StartsFromMentions(v.Mode) {
			continue
		}
		return v.ThreadContext, nil
	}
	return false, nil
}
