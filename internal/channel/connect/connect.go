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

// Drivers resolves a configured connection to something that can post.
type Drivers struct{ store *settings.Store }

func New(store *settings.Store) *Drivers { return &Drivers{store: store} }

// For reads the connection's credential and builds its driver.
//
// Read on every message rather than cached at start-up, so a rotated token
// takes effect on the next notification instead of at the next deploy — which
// matters most in the case where it was rotated because it leaked.
func (d *Drivers) For(ctx context.Context, name string) (channel.Poster, error) {
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

	build, known := drivers[conn.Kind]
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
var drivers = map[string]func(creds channel.Credentials) channel.Poster{
	"slack": func(creds channel.Credentials) channel.Poster {
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
