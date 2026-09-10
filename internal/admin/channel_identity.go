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
Who a channel account belongs to.

This is the most consequential thing anybody configures here. Binding
`U024BE7LH` to `usr_ana` grants Ana's authority — her grants, her permission to
decide an approval — to whoever holds that Slack account. Get it wrong and
somebody approves payments as somebody else, and nothing downstream can tell:
by the time a decision is being sealed, the principal is simply the principal.

So it is an administrative act with a name and a date against it, and it is
deliberately explicit. There is no matching on email, no inferring from a
display name: an account is bound because a person said so, and the trail says
which person.
*/

// KindChannelIdentity is the setting that holds one binding.
const KindChannelIdentity settings.Kind = "channel_identity"

// ErrNoAccount and ErrNoPrincipal are the two halves nothing can be inferred
// from.
var (
	ErrNoAccount   = errors.New("admin: a binding needs a channel account")
	ErrNoPrincipal = errors.New("admin: a binding needs somebody to bind it to")
)

// ChannelIdentity is one account, and who it speaks for.
type ChannelIdentity struct {
	Channel string
	// Account is the identifier the channel knows a person by, e.g. a Slack
	// user id. Never their display name, which people change.
	Account   string
	Principal domain.UserID
	// Display is who that is, for the screen. A convenience and never the key.
	Display string
	/*
		Unreadable marks a row that is stored and speaks for nobody.

		The column is JSONB, so a value that is not JSON never gets in — the
		corruption that can actually occur is JSON that parses and carries no
		principal. Listed as broken rather than shown as a binding with an
		empty name: the runtime refuses an ask on one of these and says the
		row is unreadable, so a screen displaying it as ordinary leaves an
		operator comparing an error against a row that looks fine.

		Channel and Account fall back to the setting's own key, which is
		outside the value and is what keeps the row removable however little
		of it survived.
	*/
	Unreadable bool
	/*
		Misplaced marks a binding stored somewhere it means nothing.

		A binding lives at the installation, because that is where it is read
		from: PrincipalFor asks for an exact key there, and AccountsOn refuses
		to address from anywhere else. A row at a company or an area — from a
		restore, a migration, a hand-edited settings row — therefore grants
		nobody anything.

		Listed, and marked. Hidden it would be a configuration an operator
		cannot see and cannot repair, with the only trace of it in the settings
		table; shown as ordinary it would claim an authority it does not have,
		which is the more dangerous of the two. The delete reaches it wherever
		it sits.
	*/
	Misplaced bool
	// scope is where the row actually is, so the delete can name it. Not
	// exported: it is not a fact about the binding, it is where a broken one
	// has to be reached.
	scope   domain.Scope
	scopeAt settings.ScopeKind
}

// identityKey names the setting. Both halves, because one Slack account is a
// different person in a different workspace.
func identityKey(channelName, account string) string {
	return channelName + "/" + account
}

/*
lockIdentity serialises every write about one binding.

Binding and withdrawing are the same fact from two directions, and they read
each other: the withdrawal takes an inventory of where the binding is stored,
because a copy can sit somewhere it means nothing. Read outside the write, that
inventory is a photograph — a bind landing between the two leaves a row the
withdrawal never saw, and the operator is told the account speaks for nobody
while it speaks for somebody.
*/
func lockIdentity(ctx context.Context, conn settings.DB, key string) error {
	if _, err := conn.Exec(ctx,
		`select pg_advisory_xact_lock(hashtext($1))`, "identity:"+key); err != nil {
		return fmt.Errorf("admin: lock the binding %s: %w", key, err)
	}
	return nil
}

// BindIdentity records that an account speaks for a principal.
func (c *Channels) BindIdentity(
	ctx context.Context, id ChannelIdentity, by domain.UserID,
) error {
	switch {
	case strings.TrimSpace(id.Account) == "":
		return ErrNoAccount
	case strings.TrimSpace(string(id.Principal)) == "":
		return ErrNoPrincipal
	}

	value, err := json.Marshal(map[string]string{
		"channel": id.Channel, "account": id.Account,
		"principal": string(id.Principal), "display": id.Display,
	})
	if err != nil {
		return err
	}

	key := identityKey(id.Channel, id.Account)
	guard := func(ctx context.Context, conn settings.DB) error {
		return lockIdentity(ctx, conn, key)
	}
	return writeGuarded(ctx, c.pool, c.settings, guard, folded{
		by: by, scope: domain.Scope{},
		action: "channel.identity.bound", target: key,
		set: settings.Setting{
			ScopeKind: settings.ScopeInstallation,
			Kind:      KindChannelIdentity,
			Name:      key,
			Value:     value, Enabled: true, UpdatedBy: string(by),
		},
		detail: map[string]any{
			// Both sides in the trail. "Somebody was bound to somebody" is not
			// an answer anybody can act on a year later.
			"channel": id.Channel, "account": id.Account,
			"principal": string(id.Principal),
		},
	})
}

/*
UnbindIdentity withdraws a binding, from every position it is stored in.

Every position, and this is the whole of it. A binding belongs at the
installation and a row elsewhere grants nobody anything — but both can exist at
once, under the same key, and they are one thing on the screen: one account,
one button. Removing "the one that is misplaced" then answered a request to
revoke somebody's authority by deleting the inert copy and leaving the live
one, reporting success. `PrincipalFor` went on naming them.

So the request means what an operator means by it: after this, that account
speaks for nobody. One transaction, so it does not half happen, and one event —
the act is the withdrawal, not the number of rows it took.
*/
func (c *Channels) UnbindIdentity(
	ctx context.Context, channelName, account string, by domain.UserID,
) error {
	key := identityKey(channelName, account)
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Under the binding's own lock, and the inventory is taken inside it.
	// Read before the transaction it was a photograph: a bind landing between
	// the two left a row this never saw, and the trail said the account speaks
	// for nobody while it went on speaking for somebody.
	if err := lockIdentity(ctx, tx, key); err != nil {
		return err
	}
	stored, err := c.settings.ListTx(ctx, tx, KindChannelIdentity)
	if err != nil {
		return fmt.Errorf("admin: list channel identities: %w", err)
	}

	for _, one := range stored {
		if one.Name != key {
			continue
		}
		if err := c.settings.DeleteTx(ctx, tx,
			one.ScopeKind, one.Scope, KindChannelIdentity, key); err != nil {
			return err
		}
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Action: "channel.identity.unbound", Target: key,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Identities lists every binding, for the screen that manages them.
func (c *ChannelFacts) Identities(ctx context.Context) ([]ChannelIdentity, error) {
	stored, err := c.settings.List(ctx, KindChannelIdentity)
	if err != nil {
		return nil, fmt.Errorf("admin: list channel identities: %w", err)
	}

	out := make([]ChannelIdentity, 0, len(stored))
	for _, s := range stored {
		// Where the row is, kept whatever else it turns out to be: a delete
		// recomputing the position would name the place a binding belongs
		// rather than the place this one is, and remove nothing.
		here := s.ScopeKind != settings.ScopeInstallation || s.Scope != (domain.Scope{})
		id, ok := identityFrom(s)
		if ok && id.Principal != "" {
			/*
				Named by the key, like every row here.

				PrincipalFor looks a binding up by `channel/account`, so the
				key is what grants the authority — and the same two fields
				inside the value are a copy that can only ever disagree with
				it. Published from the value, a row keyed `acme-slack/U505`
				whose contents say `old-slack/U777` grants under the first and
				is shown and deleted under the second: the console offers to
				remove a binding that is not the one doing anything.

				They are written from the same inputs, so they diverge only by
				restore or by a hand-edited row — which is exactly when the
				screen has to agree with the runtime.
			*/
			id.Channel, id.Account = keyChannel(s.Name), keyAccount(s.Name)
			id.Misplaced, id.scope, id.scopeAt = here, s.Scope, s.ScopeKind
			out = append(out, id)
			continue
		}
		// Shown as what it is. The runtime refuses an ask on this row and says
		// it is unreadable; a listing that showed it as an ordinary binding
		// with a blank name would leave an operator comparing that error
		// against a row that looks fine.
		/*
			The key names it, and the value does not get a vote.

			Falling back to the key only where the value said nothing was
			still trusting the value: a row keyed `acme-slack/U505` whose
			corrupted contents claim `old-slack` would be listed under a
			channel it does not belong to — filed on the wrong card, and
			deleted by a request naming a channel the row was never under. The
			value is the part that is broken, so on this path it is not
			evidence of anything.

			Display survives as a hint. It changes nothing and names nothing,
			and an operator deciding whether to remove a row wants every hint
			there is.
		*/
		out = append(out, ChannelIdentity{
			Channel:    keyChannel(s.Name),
			Account:    keyAccount(s.Name),
			Display:    id.Display,
			Unreadable: true,
			Misplaced:  here,
			scope:      s.Scope,
			scopeAt:    s.ScopeKind,
		})
	}
	return out, nil
}

/*
The key is what names a row, and the value does not get a vote.

PrincipalFor grants authority by `channel/account` from the key, so anything
else shown beside a binding is a second opinion about the same fact — and a
second opinion that can be wrong. On a readable row the copies inside the value
diverge only by restore or by hand; on an unreadable one the value is the
broken part outright. Either way the key is the thing that survived, because it
is not inside what broke.

One separator and the first one: a Slack account id has no slash, and the
channel is the half an operator typed.
*/
func keyChannel(key string) string {
	channelName, _, _ := strings.Cut(key, "/")
	return channelName
}

func keyAccount(key string) string {
	_, account, _ := strings.Cut(key, "/")
	return account
}

/*
PrincipalFor answers who an account speaks for, or nobody.

Nobody is the ordinary answer and not an error: most people in a workspace have
never been bound, and a message from one of them is somebody the platform does
not know rather than something going wrong. What must never happen is guessing
— an unbound account acts as no one at all.

A failure to look is the third answer, and it used to be folded into the
second. A store that was away made every account read as unbound, so an ask
would be closed telling somebody their account is not linked — which is a
sentence they would act on, about a state that was never true.
*/
func (c *ChannelFacts) PrincipalFor(
	ctx context.Context, channelName, account string,
) (domain.UserID, bool, error) {
	s, err := c.settings.Get(ctx, settings.ScopeInstallation, domain.Scope{},
		KindChannelIdentity, identityKey(channelName, account))
	if errors.Is(err, settings.ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("admin: read the binding for %s: %w", account, err)
	}
	if !s.Enabled {
		return "", false, nil
	}
	// A row that exists, is enabled and cannot be read is corrupted
	// configuration, not an ordinary absence. Answered as "nobody", it would
	// send somebody to link an account that already has a row — and the row
	// that is wrong would stay wrong, because nothing said it was there.
	id, ok := identityFrom(s)
	if !ok || id.Principal == "" {
		return "", false, fmt.Errorf(
			"admin: the binding for %s on %s is stored and unreadable", account, channelName)
	}
	return id.Principal, true, nil
}

func identityFrom(s settings.Setting) (ChannelIdentity, bool) {
	var v struct {
		Channel   string `json:"channel"`
		Account   string `json:"account"`
		Principal string `json:"principal"`
		Display   string `json:"display"`
	}
	if err := json.Unmarshal(s.Value, &v); err != nil {
		return ChannelIdentity{}, false
	}
	return ChannelIdentity{
		Channel: v.Channel, Account: v.Account,
		Principal: domain.UserID(v.Principal), Display: v.Display,
	}, true
}

/*
Secrets hands the verifier what it checks with.

Only the inbound path calls this, and only to verify — never to post. Reading a
credential out of the vault is a thing to do sparingly and in one place, which
is why it is a method here rather than a value passed around at start-up: a
rotated secret takes effect on the next request instead of at the next deploy,
which matters most in the case where it was rotated because it leaked.
*/
/*
ChannelDoor is what the unauthenticated door needs, and only that.

Slack posts to a public path, so the request has to be verified before anything
about it is believed — which needs the signing secret — and then the account it
names has to be turned into a principal. Two reads, both of them prerequisites
for trusting a stranger's request at all.

The credentials do not live on ChannelFacts because they are not a fact about
channels an ordinary reader should hold: the token that posts as this
installation, and the secret that decides which requests are genuine. A process
that lists conversations has no business being able to read either.
*/
type ChannelDoor struct{ *ChannelFacts }

func NewChannelDoor(pool *pgxpool.Pool, store *settings.Store) *ChannelDoor {
	return &ChannelDoor{ChannelFacts: NewChannelFacts(pool, store)}
}

func (c *ChannelDoor) Secrets(
	ctx context.Context, name string,
) (channel.Credentials, bool) {
	held, err := c.settings.Reveal(ctx,
		settings.ScopeInstallation, domain.Scope{}, channel.KindChannel, name)
	if err != nil || !held.Enabled {
		return channel.Credentials{}, false
	}
	return channel.ReadCredentials(held.Secret), true
}
