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
	// DirectApprovals sends an approval announced here privately as well, to
	// the people who may decide it. Outbound, like Wants: it says who is told
	// when a run stops, not what may start one.
	DirectApprovals bool
	Wants           []string
	Enabled         bool
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
			// As stored, for the reason a conversation's mode is: read through
			// the display normalisation, a connection configured by a newer
			// version comes back as HTTP, and saving any unrelated edit from
			// that reading opens the inbound door this version knows.
			DeliveryMode: channel.StoredDeliveryMode(conn.DeliveryMode),
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

/*
storedConversation is one conversation beside the row's own name.

The name is what a delete has to say. Recomputing it from the connection and
the id is how a removal reports success and removes nothing: rows written before
the connection joined the key are named by the id alone, and one of those
arrives from a restore, from a partial rollout, or from any writer still on the
version before this one.
*/
type storedConversation struct {
	name string
	conv Conversation
}

func conversationsOf(channelName string, stored []settings.Setting) []Conversation {
	rows := conversationRows(channelName, stored)
	out := make([]Conversation, 0, len(rows))
	for _, one := range rows {
		out = append(out, one.conv)
	}
	return out
}

func conversationRows(channelName string, stored []settings.Setting) []storedConversation {
	var out []storedConversation
	for _, s := range stored {
		var v struct {
			Channel         string   `json:"channel"`
			Label           string   `json:"label"`
			Mode            string   `json:"mode"`
			Sources         []string `json:"sources"`
			Agent           string   `json:"agent"`
			RunAs           string   `json:"runAs"`
			ThreadContext   bool     `json:"threadContext"`
			DirectApprovals bool     `json:"directApprovals"`
			Wants           []string `json:"wants"`
		}
		if err := json.Unmarshal(s.Value, &v); err != nil || v.Channel != channelName {
			continue
		}
		out = append(out, storedConversation{name: s.Name, conv: Conversation{
			ID: channel.ConversationID(channelName, s.Name), Label: v.Label, Scope: s.Scope,
			// As stored. Read through the display normalisation, a mode this
			// version cannot name came back as "mentions", and saving any
			// unrelated edit from that reading turned a room that started
			// nothing into one anybody could start runs from by typing in it.
			Mode:    channel.StoredMode(v.Mode),
			Sources: compactStrings(v.Sources),
			Agent:   domain.AgentID(v.Agent), RunAs: domain.UserID(v.RunAs),
			ThreadContext:   v.ThreadContext,
			DirectApprovals: v.DirectApprovals,
			Wants:           v.Wants, Enabled: s.Enabled,
		}})
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
/*
ChannelWrite is one connection as somebody asked for it to be.

An options struct because the caller's authority travels with the request and
not with the connection: whether it is needed is decided here, under the same
lock as the write, from what is attached at that moment.
*/
type ChannelWrite struct {
	Channel     Channel
	Credentials channel.Credentials
	By          domain.UserID
	// Governs says the caller holds authority over the whole installation. It
	// is only consulted when this connection carries the room for it.
	Governs bool
}

func (c *Channels) PutChannel(ctx context.Context, w ChannelWrite) error {
	ch, by := w.Channel, w.By
	if strings.TrimSpace(ch.Kind) == "" {
		return ErrNoChannelKind
	}
	if !channel.KnownDeliveryMode(ch.DeliveryMode) {
		return fmt.Errorf("%w: %q", ErrUnknownDeliveryMode, ch.DeliveryMode)
	}

	mode := channel.StoredDeliveryMode(ch.DeliveryMode)
	value, err := json.Marshal(channel.Connection{
		Kind: ch.Kind, Workspace: ch.Workspace,
		DeliveryMode: mode,
	})
	if err != nil {
		return err
	}

	base := settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      channel.KindChannel, Name: ch.Name,
		Value: value, Enabled: ch.Enabled, UpdatedBy: string(by),
	}
	guard := func(ctx context.Context, conn settings.DB) error {
		return c.guardInstallationRoom(ctx, conn, ch.Name, w.Governs)
	}
	return writeGuarded(ctx, c.pool, c.settings, guard, folded{
		by: by, scope: domain.Scope{},
		action: "channel.configured", target: ch.Name,
		set: base,
		// The credentials are folded inside the transaction, onto the row this
		// write has already locked. Read before it, a request carrying one
		// half and meaning "leave the other" is a lost update waiting for two
		// people: both read the old pair, both write their own half onto it,
		// and the second commit puts the other's back — reported as success.
		//
		// It is also where the secret is opened, which is after the guard has
		// decided whether this caller may touch the connection at all.
		fold: func(stored settings.Setting) (settings.Setting, any, error) {
			merged := mergedCredentials(
				channel.ReadCredentials(stored.Secret), w.Credentials, mode)
			set := base
			set.Secret = merged.Sealed()
			// An omitted secret means "keep what is stored", which is what
			// lets somebody change an unrelated field without pasting a token
			// back in. So a pair the mode has emptied has to say so: dropping
			// the signing secret on the way to Socket Mode, and then writing
			// nothing, left it sealed and out of sight — a credential nothing
			// verifies with, back in force the day the mode changes again,
			// while the trail recorded that it was gone.
			set.ClearSecret = merged == (channel.Credentials{})
			return set, map[string]any{
				// Never a credential, only which of them are now held. Whether
				// an installation can be spoken to is a fact an auditor may
				// need; the secret is not.
				"kind": ch.Kind, "workspace": ch.Workspace,
				"deliveryMode": mode,
				"token":        merged.Token != "", "signing": merged.Signing != "",
				"appToken": merged.AppToken != "",
			}, nil
		},
	})
}

// mergedCredentials keeps whichever half this write left out.
//
// A pure fold over what is stored. It used to read the row itself and treat
// every failure as "no such channel yet" — a vault that was away or a cancelled
// context then looked like a first write, and answered by storing the half it
// had been given as the whole of it.
func mergedCredentials(
	stored, given channel.Credentials, mode string,
) channel.Credentials {
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
	return given
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
	if conv.Scope.Company == domain.Installation && !conv.Scope.IsInstallation() {
		return ErrInstallationArea
	}
	if !channel.KnownMode(conv.Mode) {
		return fmt.Errorf("%w: %q", ErrUnknownMode, conv.Mode)
	}
	for _, want := range conv.Wants {
		if !channel.KnownEvent(channel.Event(want)) {
			return fmt.Errorf("%w: %q", ErrUnknownEvent, want)
		}
	}
	// The scope is one reason a conversation starts nothing and the mode is
	// the other, and both leave the same fields with nothing to do.
	if conv.Scope.IsInstallation() || conv.Mode == channel.ConversationAnnounce {
		conv, conv.Mode = announcesOnly(conv)
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
	// A private card rides on this conversation's own obligation: the fan-out
	// happens once a conversation has been found owed an announcement about a
	// parked run. One that never hears about them cannot send a private card
	// about one, so storing this as on would describe something the platform
	// does not do.
	//
	// The question is asked of the channel package rather than answered again
	// here. What an empty list means is its rule, and a second statement of it
	// would disagree the first time the defaults changed.
	if !channel.Wants(eventsOf(conv.Wants), channel.EventParked) {
		conv.DirectApprovals = false
	}
	value, err := json.Marshal(map[string]any{
		"channel": channelName, "label": conv.Label, "wants": conv.Wants,
		"mode": mode, "sources": sources,
		"agent": string(conv.Agent), "runAs": string(conv.RunAs),
		"threadContext":   conv.ThreadContext,
		"directApprovals": conv.DirectApprovals,
	})
	if err != nil {
		return err
	}

	// Under the connection's lock, so attaching a room and deciding whether the
	// connection may be touched cannot interleave. Whoever is allowed to write
	// this row is settled at the door; what this serialises is the pair.
	//
	// The uniqueness rule is checked here for the same reason. Read before the
	// transaction, two requests naming one conversation for two scopes both
	// saw it unmapped and both stored — and a Slack channel receiving two
	// companies' runs is the disclosure the conversation scope exists to
	// prevent.
	guard := func(ctx context.Context, conn settings.DB) error {
		if err := lockChannel(ctx, conn, channelName); err != nil {
			return err
		}
		return c.unmapped(ctx, conn, channelName, conv)
	}
	return writeGuarded(ctx, c.pool, c.settings, guard, folded{
		by: by, scope: conv.Scope,
		action: "channel.conversation.configured", target: conv.ID,
		// The row this one replaces, if it is stored under the id alone. Two
		// rows for one conversation is the ambiguity the read refuses, so the
		// older shape goes in the same act rather than being left beside its
		// replacement. Nothing is renamed ahead of time: a version before this
		// one reads the old name and only the old name, and it is still
		// serving while this one starts.
		then: func(ctx context.Context, conn settings.DB) error {
			return c.removeLegacyRow(ctx, conn, channelName, conv)
		},
		set: settings.Setting{
			ScopeKind: conversationScopeKind(conv.Scope), Scope: conv.Scope,
			Kind: channel.KindConversation,
			// The connection and the id, because either alone is ambiguous:
			// stored under the id, mapping the same one on a second connection
			// in this scope replaced the first, silently.
			Name:  channel.ConversationKey(channelName, conv.ID),
			Value: value, Enabled: conv.Enabled, UpdatedBy: string(by),
		},
		detail: map[string]any{
			"channel": channelName, "scope": conv.Scope.String(), "wants": conv.Wants,
			"mode": mode, "sources": sources,
			"agent": string(conv.Agent), "runAs": string(conv.RunAs),
			"threadContext": conv.ThreadContext,
			// Turning this on decides that a run's facts reach people privately
			// rather than only in a room somebody can be added to or removed
			// from. The trail has to say when it was turned on, and by whom.
			"directApprovals": conv.DirectApprovals,
		},
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

/*
DeleteChannel removes a connection and everything mapped into it.

The conversations go too. Leaving them would keep rows pointing at a connection
that no longer exists, which reads as configured, delivers nothing, and is
adopted by the next connection to take the name.

One transaction, holding the connection's lock. As a loop of separate
transactions it could stop halfway — conversations gone, connection still there
— and a room attached while it ran was neither seen by the authority check nor
removed by the cascade, left behind pointing at a connection that no longer
exists.
*/
func (c *Channels) DeleteChannel(
	ctx context.Context, name string, by domain.UserID, governs bool,
) error {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := c.guardInstallationRoom(ctx, tx, name, governs); err != nil {
		return err
	}
	stored, err := c.settings.ListTx(ctx, tx, channel.KindConversation)
	if err != nil {
		return fmt.Errorf("admin: list conversations: %w", err)
	}
	for _, one := range conversationRows(name, stored) {
		if err := c.settings.DeleteTx(ctx, tx, conversationScopeKind(one.conv.Scope),
			one.conv.Scope, channel.KindConversation, one.name); err != nil {
			return err
		}
		if err := Record(ctx, tx, Event{
			Principal: by, Scope: one.conv.Scope,
			Action: "channel.conversation.removed", Target: one.conv.ID,
		}); err != nil {
			return err
		}
	}

	if err := c.settings.DeleteTx(ctx, tx, settings.ScopeInstallation,
		domain.Scope{}, channel.KindChannel, name); err != nil {
		return err
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Action: "channel.removed", Target: name,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ConversationRef names one conversation: the connection it is on, the id the
// vendor calls it, and the scope it was configured in. The three travel
// together because none of them identifies it alone — and a delete keyed
// differently from the write is a removal that reports success and removes
// nothing, or somebody else's row.
type ConversationRef struct {
	Channel string
	ID      string
	Scope   domain.Scope
}

/*
removeLegacyRow deletes the same conversation stored under the id alone.

Only when it belongs to this connection. A row named by the id alone may be
somebody else's — that is the whole reason the key changed — and deleting it
because the names collide would be this defect happening one more time, in the
opposite direction.
*/
func (c *Channels) removeLegacyRow(
	ctx context.Context, conn settings.DB, channelName string, conv Conversation,
) error {
	stored, err := c.settings.ListTx(ctx, conn, channel.KindConversation)
	if err != nil {
		return fmt.Errorf("admin: list conversations: %w", err)
	}
	for _, one := range conversationRows(channelName, stored) {
		if one.name != conv.ID || one.conv.Scope != conv.Scope {
			continue
		}
		return c.settings.DeleteTx(ctx, conn, conversationScopeKind(conv.Scope),
			conv.Scope, channel.KindConversation, one.name)
	}
	return nil
}

/*
DeleteConversation stops a scope's runs reporting to a place.

The row is found before it is removed, and removed under the name it is
actually stored with. Recomputing the key instead is how a removal reports
success and removes nothing: a conversation written before the connection
joined the key is named by the id alone, and one of those arrives from a
restore, or from a writer still on the version before this one.
*/
func (c *Channels) DeleteConversation(
	ctx context.Context, ref ConversationRef, by domain.UserID,
) error {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockChannel(ctx, tx, ref.Channel); err != nil {
		return err
	}
	stored, err := c.settings.ListTx(ctx, tx, channel.KindConversation)
	if err != nil {
		return fmt.Errorf("admin: list conversations: %w", err)
	}
	for _, one := range conversationRows(ref.Channel, stored) {
		if one.conv.ID != ref.ID || one.conv.Scope != ref.Scope {
			continue
		}
		if err := c.settings.DeleteTx(ctx, tx, conversationScopeKind(ref.Scope),
			ref.Scope, channel.KindConversation, one.name); err != nil {
			return err
		}
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: ref.Scope,
		Action: "channel.conversation.removed", Target: ref.ID,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ErrConversationMapped means this conversation already speaks for another
// scope on this connection.
var ErrConversationMapped = errors.New("admin: that conversation already speaks for another scope")

// unmapped refuses a conversation that already belongs to a different scope.
//
// The same scope is not a conflict: pointing a conversation at the scope it is
// already pointed at is how somebody renames it or changes which events it
// wants.
func (c *Channels) unmapped(
	ctx context.Context, conn settings.DB, channelName string, conv Conversation,
) error {
	existing, err := c.settings.ListTx(ctx, conn, channel.KindConversation)
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

// eventsOf reads the stored event names as the channel package's own type.
// The names are that package's vocabulary; this only carries them across.
func eventsOf(names []string) []channel.Event {
	out := make([]channel.Event, 0, len(names))
	for _, name := range names {
		out = append(out, channel.Event(name))
	}
	return out
}
