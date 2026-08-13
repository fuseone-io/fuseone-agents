package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
Configuring where runs report.

Two settings rather than one, because they have different secrets and different
scopes. A connection holds the bot credential and belongs to the installation;
a conversation holds nothing secret and belongs to the scope whose runs it
carries — which is the governing part, and the reason a conversation cannot be
declared without one.
*/

var (
	ErrNoChannelKind = errors.New("admin: a channel needs a kind")
	ErrNoCompany     = errors.New("admin: a conversation belongs to a scope")
)

// Channels reads and writes channel configuration, recording each change.
type Channels struct {
	pool     *pgxpool.Pool
	settings *settings.Store
}

func NewChannels(pool *pgxpool.Pool, store *settings.Store) *Channels {
	return &Channels{pool: pool, settings: store}
}

// Channel is a connection and the conversations inside it.
type Channel struct {
	Name          string
	Kind          string
	Workspace     string
	Enabled       bool
	HasCredential bool
	Conversations []Conversation
}

// Conversation is one place, and the scope it speaks for.
type Conversation struct {
	ID      string
	Label   string
	Scope   domain.Scope
	Wants   []string
	Enabled bool
}

// List answers with every connection and the conversations mapped into it.
func (c *Channels) List(ctx context.Context) ([]Channel, error) {
	connections, err := c.settings.List(ctx, channel.KindChannel)
	if err != nil {
		return nil, fmt.Errorf("admin: list channels: %w", err)
	}
	conversations, err := c.settings.List(ctx, channel.KindConversation)
	if err != nil {
		return nil, fmt.Errorf("admin: list conversations: %w", err)
	}

	out := make([]Channel, 0, len(connections))
	for _, s := range connections {
		var conn channel.Connection
		_ = json.Unmarshal(s.Value, &conn)
		out = append(out, Channel{
			Name: s.Name, Kind: conn.Kind, Workspace: conn.Workspace,
			Enabled: s.Enabled, HasCredential: s.HasSecret,
			Conversations: conversationsOf(s.Name, conversations),
		})
	}
	return out, nil
}

func conversationsOf(channelName string, stored []settings.Setting) []Conversation {
	var out []Conversation
	for _, s := range stored {
		var v struct {
			Channel string   `json:"channel"`
			Label   string   `json:"label"`
			Wants   []string `json:"wants"`
		}
		if err := json.Unmarshal(s.Value, &v); err != nil || v.Channel != channelName {
			continue
		}
		out = append(out, Conversation{
			ID: s.Name, Label: v.Label, Scope: s.Scope,
			Wants: v.Wants, Enabled: s.Enabled,
		})
	}
	return out
}

// PutChannel configures a connection, sealing its credential.
func (c *Channels) PutChannel(ctx context.Context, ch Channel, token string, by domain.UserID) error {
	if strings.TrimSpace(ch.Kind) == "" {
		return ErrNoChannelKind
	}

	value, err := json.Marshal(channel.Connection{Kind: ch.Kind, Workspace: ch.Workspace})
	if err != nil {
		return err
	}

	return writeSetting(ctx, c.pool, c.settings, by, domain.Scope{}, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      channel.KindChannel, Name: ch.Name,
		Value: value, Secret: token, Enabled: ch.Enabled, UpdatedBy: string(by),
	}, "channel.configured", ch.Name, map[string]any{
		// Never the credential, only that one arrived.
		"kind": ch.Kind, "workspace": ch.Workspace, "credential": token != "",
	})
}

// PutConversation points a scope's runs at a conversation.
func (c *Channels) PutConversation(
	ctx context.Context, channelName string, conv Conversation, by domain.UserID,
) error {
	if conv.Scope.Company == "" {
		return ErrNoCompany
	}

	value, err := json.Marshal(map[string]any{
		"channel": channelName, "label": conv.Label, "wants": conv.Wants,
	})
	if err != nil {
		return err
	}

	kind := settings.ScopeCompany
	if conv.Scope.Area != "" {
		kind = settings.ScopeArea
	}

	return writeSetting(ctx, c.pool, c.settings, by, conv.Scope, settings.Setting{
		ScopeKind: kind, Scope: conv.Scope,
		Kind: channel.KindConversation, Name: conv.ID,
		Value: value, Enabled: conv.Enabled, UpdatedBy: string(by),
	}, "channel.conversation.configured", conv.ID, map[string]any{
		"channel": channelName, "scope": conv.Scope.String(), "wants": conv.Wants,
	})
}

// DeleteChannel removes a connection and everything mapped into it.
//
// The conversations go too. Leaving them would keep rows pointing at a
// connection that no longer exists, which reads as configured and delivers
// nothing.
func (c *Channels) DeleteChannel(ctx context.Context, name string, by domain.UserID) error {
	existing, err := c.settings.List(ctx, channel.KindConversation)
	if err != nil {
		return fmt.Errorf("admin: list conversations: %w", err)
	}
	for _, conv := range conversationsOf(name, existing) {
		if err := c.DeleteConversation(ctx, conv.ID, conv.Scope, by); err != nil {
			return err
		}
	}

	return removeSetting(ctx, c.pool, c.settings, by, domain.Scope{},
		channel.KindChannel, name, "channel.removed")
}

// DeleteConversation stops a scope's runs reporting to a place.
func (c *Channels) DeleteConversation(
	ctx context.Context, id string, scope domain.Scope, by domain.UserID,
) error {
	at := settings.ScopeCompany
	if scope.Area != "" {
		at = settings.ScopeArea
	}
	return removeScopedSetting(ctx, c.pool, c.settings, by, at, scope, scope,
		channel.KindConversation, id, "channel.conversation.removed")
}
