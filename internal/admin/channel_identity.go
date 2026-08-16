package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
}

// identityKey names the setting. Both halves, because one Slack account is a
// different person in a different workspace.
func identityKey(channelName, account string) string {
	return channelName + "/" + account
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

	return writeSetting(ctx, c.pool, c.settings, by, domain.Scope{}, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      KindChannelIdentity,
		Name:      identityKey(id.Channel, id.Account),
		Value:     value, Enabled: true, UpdatedBy: string(by),
	}, "channel.identity.bound", identityKey(id.Channel, id.Account),
		map[string]any{
			// Both sides in the trail. "Somebody was bound to somebody" is not
			// an answer anybody can act on a year later.
			"channel": id.Channel, "account": id.Account,
			"principal": string(id.Principal),
		})
}

// UnbindIdentity withdraws a binding.
func (c *Channels) UnbindIdentity(
	ctx context.Context, channelName, account string, by domain.UserID,
) error {
	return removeSetting(ctx, c.pool, c.settings, by, domain.Scope{},
		KindChannelIdentity, identityKey(channelName, account),
		"channel.identity.unbound")
}

// Identities lists every binding, for the screen that manages them.
func (c *Channels) Identities(ctx context.Context) ([]ChannelIdentity, error) {
	stored, err := c.settings.List(ctx, KindChannelIdentity)
	if err != nil {
		return nil, fmt.Errorf("admin: list channel identities: %w", err)
	}

	out := make([]ChannelIdentity, 0, len(stored))
	for _, s := range stored {
		id, ok := identityFrom(s)
		if ok && id.Principal != "" {
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
		})
	}
	return out, nil
}

/*
The key is what names a row whose value cannot be trusted to.

Canonical rather than a fallback. A value that kept a channel and lost an
account would otherwise have the recovered account discarded with it, and a
value that kept a *wrong* channel would move the row to a card it does not
belong on — where the console would then delete it by naming a channel it was
never under. On an unreadable row the value is the broken part; the key is the
only thing that survived intact, because it is not inside the value.

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
func (c *Channels) PrincipalFor(
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
func (c *Channels) Secrets(
	ctx context.Context, name string,
) (channel.Credentials, bool) {
	held, err := c.settings.Reveal(ctx,
		settings.ScopeInstallation, domain.Scope{}, channel.KindChannel, name)
	if err != nil || !held.Enabled {
		return channel.Credentials{}, false
	}
	return channel.ReadCredentials(held.Secret), true
}
