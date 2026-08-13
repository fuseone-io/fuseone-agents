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
	if s.Secret == "" {
		return nil, fmt.Errorf("connect: channel %q has no credential", name)
	}

	var conn channel.Connection
	if err := json.Unmarshal(s.Value, &conn); err != nil {
		return nil, fmt.Errorf("connect: read connection %q: %w", name, err)
	}

	switch conn.Kind {
	case "slack":
		return slack.New(s.Secret), nil
	default:
		// Named rather than ignored. A connection of a kind nobody has built
		// should say so, not fail as a notification that never arrives.
		return nil, fmt.Errorf("connect: channel %q is of an unsupported kind %q", name, conn.Kind)
	}
}
