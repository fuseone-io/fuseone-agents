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
	ErrNoWatchSource = errors.New("admin: watched messages need at least one source")
	ErrNoWatchAgent  = errors.New("admin: watched messages need an agent to start")
	ErrNoWatchRunAs  = errors.New("admin: watched messages need a principal to run as")
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
	DeliveryMode  string
	Enabled       bool
	HasCredential bool
	// HasSigning reports whether the inbound half is configured, without
	// exposing it. A channel can post long before it can be answered.
	HasSigning bool
	// HasAppToken reports whether Slack Socket Mode can be opened without
	// revealing the app-level token that opens it.
	HasAppToken   bool
	Conversations []Conversation
}

// Conversation is one place, and the scope it speaks for.
type Conversation struct {
	ID            string
	Label         string
	Scope         domain.Scope
	Mode          string
	Sources       []string
	Agent         domain.AgentID
	RunAs         domain.UserID
	ThreadContext bool
	Wants         []string
	Enabled       bool
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
		// Whether the inbound half is on, without revealing either secret. It
		// needs the sealed value, so it is read here rather than inferred.
		secret := c.channelSecretState(ctx, s.Name)
		out = append(out, Channel{
			Name: s.Name, Kind: conn.Kind, Workspace: conn.Workspace,
			DeliveryMode: channel.DeliveryMode(conn.DeliveryMode),
			Enabled:      s.Enabled, HasCredential: secret.BotToken,
			HasSigning: secret.Signing, HasAppToken: secret.AppToken,
			Conversations: conversationsOf(s.Name, conversations),
		})
	}
	return out, nil
}

type channelSecretState struct {
	BotToken bool
	Signing  bool
	AppToken bool
}

// channelSecretState answers which sealed pieces exist, without exposing any
// of them. A single HasSecret bit is no longer enough: posting, HTTP inbound
// and Socket Mode are three different capabilities.
func (c *Channels) channelSecretState(ctx context.Context, name string) channelSecretState {
	held, err := c.settings.Reveal(ctx,
		settings.ScopeInstallation, domain.Scope{}, channel.KindChannel, name)
	if err != nil {
		return channelSecretState{}
	}
	creds := channel.ReadCredentials(held.Secret)
	return channelSecretState{
		BotToken: creds.Token != "",
		Signing:  creds.Signing != "",
		AppToken: creds.AppToken != "",
	}
}

func conversationsOf(channelName string, stored []settings.Setting) []Conversation {
	var out []Conversation
	for _, s := range stored {
		var v struct {
			Channel       string   `json:"channel"`
			Label         string   `json:"label"`
			Mode          string   `json:"mode"`
			Sources       []string `json:"sources"`
			Agent         string   `json:"agent"`
			RunAs         string   `json:"runAs"`
			ThreadContext bool     `json:"threadContext"`
			Wants         []string `json:"wants"`
		}
		if err := json.Unmarshal(s.Value, &v); err != nil || v.Channel != channelName {
			continue
		}
		out = append(out, Conversation{
			ID: s.Name, Label: v.Label, Scope: s.Scope,
			Mode:    channel.ConversationMode(v.Mode),
			Sources: compactStrings(v.Sources),
			Agent:   domain.AgentID(v.Agent), RunAs: domain.UserID(v.RunAs),
			ThreadContext: v.ThreadContext,
			Wants:         v.Wants, Enabled: s.Enabled,
		})
	}
	return out
}

/*
PutChannel configures a connection, sealing its credentials.

The bot token may be omitted to keep the stored one. The inbound secret belongs
to the selected delivery mode: HTTP keeps a signing secret, Socket Mode keeps
an app-level token, and switching modes drops the other one rather than hiding
an unused credential in the vault.
*/
func (c *Channels) PutChannel(
	ctx context.Context, ch Channel, creds channel.Credentials, by domain.UserID,
) error {
	if strings.TrimSpace(ch.Kind) == "" {
		return ErrNoChannelKind
	}

	mode := channel.DeliveryMode(ch.DeliveryMode)
	value, err := json.Marshal(channel.Connection{
		Kind: ch.Kind, Workspace: ch.Workspace,
		DeliveryMode: mode,
	})
	if err != nil {
		return err
	}

	merged, err := c.mergeCredentials(ctx, ch.Name, creds, mode)
	if err != nil {
		return err
	}

	return writeSetting(ctx, c.pool, c.settings, by, domain.Scope{}, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      channel.KindChannel, Name: ch.Name,
		Value: value, Secret: merged.Sealed(), Enabled: ch.Enabled, UpdatedBy: string(by),
	}, "channel.configured", ch.Name, map[string]any{
		// Never a credential, only which of them are now held. Whether an
		// installation can be spoken to is a fact an auditor may need; the
		// secret is not.
		"kind": ch.Kind, "workspace": ch.Workspace,
		"deliveryMode": mode,
		"token":        merged.Token != "", "signing": merged.Signing != "",
		"appToken": merged.AppToken != "",
	})
}

// mergeCredentials keeps whichever half this write left out.
func (c *Channels) mergeCredentials(
	ctx context.Context, name string, given channel.Credentials, mode string,
) (channel.Credentials, error) {
	held, err := c.settings.Reveal(ctx,
		settings.ScopeInstallation, domain.Scope{}, channel.KindChannel, name)
	if err != nil {
		// No such channel yet: this write is the first, and what it carries is
		// all there is.
		return channelCredentialsForMode(given, mode), nil //nolint:nilerr // absent is not a failure here
	}

	stored := channel.ReadCredentials(held.Secret)
	if given.Token == "" {
		given.Token = stored.Token
	}
	switch mode {
	case channel.DeliverySocket:
		if given.AppToken == "" {
			given.AppToken = stored.AppToken
		}
		given.Signing = ""
	default:
		if given.Signing == "" {
			given.Signing = stored.Signing
		}
		given.AppToken = ""
	}
	return given, nil
}

func channelCredentialsForMode(creds channel.Credentials, mode string) channel.Credentials {
	if mode == channel.DeliverySocket {
		creds.Signing = ""
		return creds
	}
	creds.AppToken = ""
	return creds
}

/*
PutConversation points a scope's runs at a conversation.

A conversation speaks for one scope. Mapped into two, an ask arriving in it
would be governed by whichever row a query returned first — the same message
judged differently on different days, and nobody able to answer "who could have
asked for this" (NT-005 §4).

Refused here so the screen cannot make the configuration, and refused again
when it is read: a row can also arrive by restore, by migration, or from a
version of this that did not check, and the runtime must not trust a rule it
only enforces on the way in.
*/
func (c *Channels) PutConversation(
	ctx context.Context, channelName string, conv Conversation, by domain.UserID,
) error {
	if conv.Scope.Company == "" {
		return ErrNoCompany
	}
	mode := channel.ConversationMode(conv.Mode)
	sources := compactStrings(conv.Sources)
	conv.Agent = domain.AgentID(strings.TrimSpace(string(conv.Agent)))
	if channel.StartsFromWatch(mode) {
		switch {
		case len(sources) == 0:
			return ErrNoWatchSource
		case conv.Agent == "":
			return ErrNoWatchAgent
		case strings.TrimSpace(string(conv.RunAs)) == "":
			return ErrNoWatchRunAs
		}
	} else {
		// The agent survives, because it says which agent this conversation is
		// for and a mention there needs no name. The principal and the sources
		// do not: a mention runs as the person whose account is bound, so a
		// RunAs on a conversation that watches nothing is a delegation nothing
		// consumes and nobody can explain later.
		sources = nil
		conv.RunAs = ""
	}
	if !channel.StartsFromMentions(mode) {
		conv.ThreadContext = false
	}
	if err := c.unmapped(ctx, channelName, conv); err != nil {
		return err
	}

	value, err := json.Marshal(map[string]any{
		"channel": channelName, "label": conv.Label, "wants": conv.Wants,
		"mode": mode, "sources": sources,
		"agent": string(conv.Agent), "runAs": string(conv.RunAs),
		"threadContext": conv.ThreadContext,
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
		"mode": mode, "sources": sources,
		"agent": string(conv.Agent), "runAs": string(conv.RunAs),
		"threadContext": conv.ThreadContext,
	})
}

func compactStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, one := range in {
		one = strings.TrimSpace(one)
		if one != "" {
			out = append(out, one)
		}
	}
	return out
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

// ErrConversationMapped means this conversation already speaks for another
// scope on this connection.
var ErrConversationMapped = errors.New("admin: that conversation already speaks for another scope")

// unmapped refuses a conversation that already belongs to a different scope.
//
// The same scope is not a conflict: pointing a conversation at the scope it is
// already pointed at is how somebody renames it or changes which events it
// wants.
func (c *Channels) unmapped(ctx context.Context, channelName string, conv Conversation) error {
	existing, err := c.settings.List(ctx, channel.KindConversation)
	if err != nil {
		return fmt.Errorf("admin: list conversations: %w", err)
	}
	for _, one := range conversationsOf(channelName, existing) {
		if one.ID == conv.ID && one.Scope != conv.Scope {
			return fmt.Errorf("%w: %s speaks for %s", ErrConversationMapped, conv.ID, one.Scope)
		}
	}
	return nil
}
