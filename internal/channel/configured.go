package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
	/*
		ConversationAnnounce means nothing said here starts anything.

		A room somebody added the bot to for visibility is not a room anybody
		should be able to start a run from by typing in it, and saying so is
		better than a conversation whose stored mode describes an inbound path
		it does not have.

		It is also the only mode a conversation for the whole installation may
		have. That scope contains every company, and containment is right for
		hearing and wrong for asking (origin.go): a room that hears about every
		company is a reasonable thing to configure, and one that can start an
		agent in every company is a different grant entirely.
	*/
	ConversationAnnounce = "announce"
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

/*
DeliveryMode normalises for a screen. It answers HTTP for anything it does not
recognise, which is right for a label and wrong for a door — see the pair below.
*/
func DeliveryMode(mode string) string {
	if mode == DeliverySocket {
		return DeliverySocket
	}
	return DeliveryHTTP
}

// StoredDeliveryMode is the value as configured, with the one translation that
// is defined: empty was HTTP, from before Socket Mode existed. A value this
// version cannot name travels intact, so an edit cannot quietly turn it into
// one this version does act on.
func StoredDeliveryMode(mode string) string {
	if mode == "" {
		return DeliveryHTTP
	}
	return mode
}

// KnownDeliveryMode answers whether this version understands a stored delivery
// mode at all. Refused on the way in rather than normalised, for the reason
// KnownMode is: normalising is how a connection configured by a newer version
// comes back as one whose inbound door this version opens.
func KnownDeliveryMode(mode string) bool {
	return mode == "" || mode == DeliveryHTTP || mode == DeliverySocket
}

/*
DeliversOverHTTP answers whether Slack reaches this installation through the
public webhook path.

An allowlist, and read from the stored value. Written as "anything that is not
socket", a mode this version does not know opens the HTTP door — the same
fail-open the conversation modes had, one layer down: a restored connection
would start accepting asks again, verified by whichever signing secret was
sealed beside it.
*/
func DeliversOverHTTP(mode string) bool {
	return mode == "" || mode == DeliveryHTTP
}

// DeliversOverSocket answers whether the worker opens Socket Mode outbound.
// Empty is not one of them: socket had to be asked for from the day it existed.
func DeliversOverSocket(mode string) bool {
	return mode == DeliverySocket
}

func ConversationMode(mode string) string {
	switch mode {
	case ConversationWatch:
		return ConversationWatch
	case ConversationBoth:
		return ConversationBoth
	case ConversationAnnounce:
		return ConversationAnnounce
	default:
		// Empty is a conversation configured before modes existed, and it took
		// mentions. Anything else is a value this version does not know, and
		// ConversationMode is not where that is decided — the predicates below
		// name what may start a run, and neither of them names this.
		return ConversationMentions
	}
}

/*
StartsFromMentions answers whether a person mentioning the bot may start a run.

An allowlist, and read from the stored value rather than the normalised one.
Written as "anything that is not watch", the answer for a mode nobody has added
yet is yes — so the day a mode is named for a room that starts nothing, it
starts runs and every existing test goes on passing.

Normalising first would lose the distinction that matters here: ConversationMode
answers unknown with mentions, which is right for a screen and wrong for this.
Empty is named explicitly, because it is the one value that legitimately means
mentions — a conversation configured before modes existed. Anything else this
version does not recognise starts nothing.
*/
func StartsFromMentions(mode string) bool {
	return mode == "" || mode == ConversationMentions || mode == ConversationBoth
}

// StartsFromWatch answers whether a configured message source may start a run.
// An allowlist for the same reason, and empty is not one of them: watching had
// to be asked for from the day it existed.
func StartsFromWatch(mode string) bool {
	return mode == ConversationWatch || mode == ConversationBoth
}

/*
StoredMode is the mode as configured, with the one translation that is defined.

Empty means mentions — a conversation configured before modes existed — and
nothing else is translated. A value this version cannot name travels intact:
read as "mentions" it would come back through an edit as a room that starts
runs, which is the same fail-open the runtime already refuses, arriving by the
console instead of by the door.
*/
func StoredMode(mode string) string {
	if mode == "" {
		return ConversationMentions
	}
	return mode
}

/*
KnownMode answers whether this version understands a stored mode at all.

Asked on the way in, where the alternative is worse than at the read: an
operator editing on an older console would turn a room a newer version had set
to start nothing into one that starts runs by mention, and the trail would
record an ordinary edit. Empty is known — it is a conversation configured
before modes existed.
*/
func KnownMode(mode string) bool {
	switch mode {
	case "", ConversationMentions, ConversationWatch,
		ConversationBoth, ConversationAnnounce:
		return true
	}
	return false
}

// startsSomething answers whether anything at all may start a run here.
//
// The union of the two allowlists rather than a third list, so a mode added to
// one of them cannot be forgotten here — and a mode named in neither starts
// nothing, whether it is "announce" or a value written by a newer version.
func startsSomething(mode string) bool {
	return StartsFromMentions(mode) || StartsFromWatch(mode)
}

/*
Where a conversation is stored, and how a reader knows which shape it is in.

A conversation belongs to a connection, and the key said only the id: two
workspaces are two namespaces, so mapping the same id at one scope on a second
connection replaced the first — silently, because the write that did it looked
like an ordinary configuration.

Moving the key was a two-release act, and this is the second half: **this
version reads both shapes and writes the new one**, and the migration renames
what the first half left behind. Reading both is not leftover politeness — the
previous version is still serving while this one rolls out, it goes on writing
the old shape, and a row also arrives from a restore taken before the move. The
old shape is therefore read for as long as anything can produce one, which is
longer than the rollout.

The shape is declared by the row and never inferred from the name. A stored id
may look like anything a vendor chose — a Teams conversation id begins with
digits and a colon — so a reader deciding by appearance would take somebody's id
apart and answer as a different conversation.
*/

const (
	// KeyVersionName is a row whose name is the conversation id alone. Absent
	// from the value, which is how every row written before the move reads
	// back — and rows in that shape still arrive, from a pod on the previous
	// version and from a restore taken before it.
	KeyVersionName = 0
	// KeyVersionConnection is a row whose name holds the connection as well as
	// the id. What this version writes, and what the migration renames the old
	// rows into.
	KeyVersionConnection = 2
)

/*
ConversationKey is the name a row of that version is stored under.

Length-prefixed, for the reason AskKey is: joined with a separator alone, a
connection called "workspace" holding "team/C" and one called "workspace/team"
holding "C" produce the same string, and in one scope that is one row. Both
halves are names somebody typed or a vendor chose, so neither can be promised
free of the separator; a length cannot be forged by punctuation.

Every conversation this version stores is named by it, and the migration renames
the older rows into the same shape. A reader still meets the old shape, so the
name alone never decides which it is looking at — the row says so.
*/
func ConversationKey(channelName, id string) string {
	return strconv.Itoa(len(channelName)) + ":" + channelName + "/" + id
}

/*
ConversationIDOf answers which conversation a stored row is for.

The row's own declared version decides, and the connection comes from the row
rather than from the name. A row claiming the new shape without carrying it is
illegible rather than read as something else: it is nobody's conversation, and
guessing which one is how a configuration ends up governing a message it was
never written for.
*/
func ConversationIDOf(keyVersion int, channelName, name string) (string, bool) {
	// An allowlist of exactly the versions this binary knows. Read as ranges,
	// a version from the future fell into whichever side it happened to be
	// nearer — so a row a later release wrote in a shape nobody here has seen
	// would have been taken apart as though it were v2, and answered as some
	// other conversation.
	switch keyVersion {
	case KeyVersionName:
		return name, true
	case KeyVersionConnection:
		prefix := strconv.Itoa(len(channelName)) + ":" + channelName + "/"
		id, found := strings.CutPrefix(name, prefix)
		if !found || id == "" {
			return "", false
		}
		return id, true
	default:
		return "", false
	}
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
	// KeyVersion is the shape of the row's own name, declared rather than
	// guessed. Absent means the name is the conversation id.
	KeyVersion int `json:"keyVersion,omitempty"`
	// Mode governs inbound starts. Wants governs outbound announcements; they
	// are deliberately separate decisions.
	Mode          string         `json:"mode,omitempty"`
	Sources       []string       `json:"sources,omitempty"`
	Agent         domain.AgentID `json:"agent,omitempty"`
	RunAs         domain.UserID  `json:"runAs,omitempty"`
	ThreadContext bool           `json:"threadContext,omitempty"`
	// DirectApprovals is outbound, like Wants and unlike Mode: it says an
	// approval announced here also reaches the people who may decide it,
	// privately.
	DirectApprovals bool    `json:"directApprovals,omitempty"`
	Wants           []Event `json:"wants,omitempty"`
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
		id, legible := ConversationIDOf(v.KeyVersion, v.Channel, s.Name)
		if !legible {
			continue
		}
		out = append(out, Conversation{
			Channel: v.Channel, ID: id, Label: v.Label,
			Agent: v.Agent, Wants: v.Wants,
			DirectApprovals: v.DirectApprovals,
		})
	}
	return out, nil
}

/*
EnabledConnections answers which connections this installation can speak from.

Only asked when an agent wants its approvals sent privately and no conversation
names a workspace. Disabled ones are left out because a connection somebody
switched off is configuration that exists and is not in force — and a run
announced from it would be a message nobody receives, recorded as sent.
*/
func (c *Configured) EnabledConnections(ctx context.Context) ([]string, error) {
	stored, err := c.store.List(ctx, KindChannel)
	if err != nil {
		return nil, fmt.Errorf("channel: list connections: %w", err)
	}
	var out []string
	for _, s := range stored {
		// Where a connection is stored is part of what it is: they are
		// installation-wide, and the credential is sealed there. A row at a
		// company or an area is not a second connection — it is something a
		// restore or a hand edit left behind, and counting it would make a
		// healthy installation look like it has two workspaces to choose
		// between, which is answered by telling nobody.
		if s.Enabled && s.ScopeKind == settings.ScopeInstallation && s.Scope == (domain.Scope{}) {
			out = append(out, s.Name)
		}
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

	found := conversationsNamed(stored, channelName, id)
	// Every row for this conversation, and only then a decision. Answering
	// from the first one found made row order decide which configuration was
	// in force: the same pair Resolve reports as ambiguous started the agent
	// here, under whichever principal the database happened to return first.
	if len(found) != 1 {
		return WatchRule{}, false, nil
	}

	one := found[0]
	// The door asks this before the consumer resolves anything, so a row for
	// the whole installation would let any configured source write an inbox
	// row carrying a configured principal — refused a sweep later, after the
	// write and the delegation had already travelled. The mode alone would not
	// catch a restored row that says watch.
	if one.scope.IsInstallation() || !StartsFromWatch(one.value.Mode) {
		return WatchRule{}, false, nil
	}
	v := one.value
	if v.Agent == "" || v.RunAs == "" || !source.Matches(v.Sources) {
		return WatchRule{}, false, nil
	}
	return WatchRule{Agent: v.Agent, RunAs: v.RunAs, Sources: v.Sources}, true, nil
}

// storedConversation is one row read back, kept with the scope it was stored
// in: the scope is administrative and nothing inside the value may widen it.
type storedConversation struct {
	scope domain.Scope
	value conversationValue
}

// conversationsNamed collects every enabled row for one conversation on one
// connection. Two of them is a configuration nobody can act on, and saying so
// is the caller's job — the count is the answer here.
func conversationsNamed(
	stored []settings.Setting, channelName, id string,
) []storedConversation {
	var found []storedConversation
	for _, s := range stored {
		if !s.Enabled {
			continue
		}
		var v conversationValue
		if err := json.Unmarshal(s.Value, &v); err != nil {
			// One malformed row must not decide for the others, and must not
			// hide them either: it is not counted, so a legible row beside it
			// still answers.
			continue
		}
		// The connection first, then the id the row says it is for.
		if v.Channel != channelName {
			continue
		}
		stored, legible := ConversationIDOf(v.KeyVersion, v.Channel, s.Name)
		if !legible || stored != id {
			continue
		}
		found = append(found, storedConversation{scope: s.Scope, value: v})
	}
	return found
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
	for _, one := range conversationsNamed(stored, channelName, id) {
		if !StartsFromMentions(one.value.Mode) {
			continue
		}
		return one.value.ThreadContext, nil
	}
	return false, nil
}
