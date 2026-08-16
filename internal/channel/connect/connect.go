/*
Package connect builds the bot that speaks for a configured channel.

It sits below `channel` rather than inside it because a vendor package imports
the channel types, so the channel package cannot import a vendor package back.
That is the import cycle telling the truth about the layering: the types are
the contract, the vendors implement it, and choosing between them is a third
job that knows both.
*/
package connect

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/channel/slack"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
Driver is what a vendor has to be able to do to be a channel here.

Posting and saying, both required. Listing conversations is not, because a
vendor that cannot list them is a real difference the console renders as "type
the identifier" — but a vendor that cannot say a sentence back is not a channel
at all, and a table that let one in would compile and then leave every refusal
undeliverable on that connection.
*/
type Driver interface {
	channel.Poster
	Say(ctx context.Context, conversation, thread, text string) error
}

// Drivers resolves a configured connection to something that can post.
type Drivers struct {
	store *settings.Store
	build map[string]func(channel.Credentials) Driver
}

func New(store *settings.Store) *Drivers { return newWith(store, drivers) }

// newWith is the same thing over another table, so a test can see which
// credential a name resolved to without a fake vendor over HTTP.
func newWith(
	store *settings.Store, table map[string]func(channel.Credentials) Driver,
) *Drivers {
	return &Drivers{store: store, build: table}
}

// For reads the connection's credential and builds its driver.
//
// Read on every message rather than cached at start-up, so a rotated token
// takes effect on the next notification instead of at the next deploy — which
// matters most in the case where it was rotated because it leaked.
func (d *Drivers) For(ctx context.Context, name string) (channel.Poster, error) {
	return d.driver(ctx, name)
}

/*
Reply says a sentence in the thread an ask was made in.

This is what satisfies the consumer's Answers: it holds asks from every
connection at once and names the one each refusal belongs to, and resolving
that name is this package's whole job.

Every failure here is an error rather than a shrug, because the caller reads it
as "still owed". A channel switched off, a credential rotated away, a vendor
that was down: all three reverse, and a person hearing their refusal late is a
better outcome than the platform recording that it told them.
*/
func (d *Drivers) Reply(ctx context.Context, name, conversation, thread, text string) error {
	driver, err := d.driver(ctx, name)
	if err != nil {
		return err
	}
	return driver.Say(ctx, conversation, thread, text)
}

func (d *Drivers) driver(ctx context.Context, name string) (Driver, error) {
	s, err := d.store.Reveal(ctx,
		settings.ScopeInstallation, domain.Scope{}, channel.KindChannel, name)
	if err != nil {
		return nil, fmt.Errorf("connect: read connection: %w", err)
	}
	if !s.Enabled {
		return nil, fmt.Errorf("connect: channel %q is switched off", name)
	}
	var conn channel.Connection
	if err := json.Unmarshal(s.Value, &conn); err != nil {
		return nil, fmt.Errorf("connect: read connection %q: %w", name, err)
	}

	creds := channel.ReadCredentials(s.Secret)
	if creds.Token == "" {
		return nil, fmt.Errorf("connect: channel %q has no credential", name)
	}

	build, known := d.build[conn.Kind]
	if !known {
		// Named rather than ignored. A connection of a kind nobody has built
		// should say so, not fail as a notification that never arrives.
		return nil, fmt.Errorf("connect: channel %q is of an unsupported kind %q", name, conn.Kind)
	}
	return build(creds), nil
}

/*
drivers is every vendor this binary can talk to.

A table rather than a switch, because the console asks what it may offer and
the answer has to be the same list that builds the connection. Two places
saying which vendors exist is one place offering a kind the binary cannot make.
*/
var drivers = map[string]func(creds channel.Credentials) Driver{
	"slack": func(creds channel.Credentials) Driver {
		driver := slack.New(creds.Token)
		if creds.Signing != "" {
			driver = driver.Decidable()
		}
		return driver
	},
}

// Kinds is what this installation can connect, sorted so the console offers
// them in a stable order.
func (d *Drivers) Kinds() []string { return Kinds() }

// Kinds is the same list, without needing a configured store to ask.
func Kinds() []string {
	out := make([]string, 0, len(drivers))
	for kind := range drivers {
		out = append(out, kind)
	}
	slices.Sort(out)
	return out
}

/*
Conversations answers what a connection can be pointed at.

Only the driver knows how, and not every driver can: a channel with no listing
API answers with a refusal the console shows as "type the identifier" rather
than pretending the bot is in nothing.
*/
func (d *Drivers) Conversations(
	ctx context.Context, name string,
) ([]channel.Available, error) {
	driver, err := d.For(ctx, name)
	if err != nil {
		return nil, err
	}
	lister, ok := driver.(interface {
		Conversations(context.Context) ([]slack.Conversation, error)
	})
	if !ok {
		return nil, fmt.Errorf("connect: channel %q cannot list its conversations", name)
	}

	found, err := lister.Conversations(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]channel.Available, 0, len(found))
	for _, c := range found {
		out = append(out, channel.Available{ID: c.ID, Name: c.Name, Private: c.Private})
	}
	return out, nil
}
