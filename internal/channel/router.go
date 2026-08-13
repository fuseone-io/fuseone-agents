package channel

import (
	"context"
	"fmt"
)

/*
One reporter, several workspaces.

A Poster speaks as one bot with one token. An installation may have a Slack
workspace for one company and a Teams tenant for another, and a conversation
names the connection it belongs to — so the thing the reporter holds routes,
and the thing that talks is chosen per message.

Resolving per post rather than at start-up is what lets a token be rotated
without restarting the workers.
*/
type Router struct{ drivers Drivers }

// Drivers answers which bot speaks for a connection.
type Drivers interface {
	For(ctx context.Context, channel string) (Poster, error)
}

func NewRouter(d Drivers) *Router { return &Router{drivers: d} }

func (r *Router) Post(ctx context.Context, c Conversation, m Message) (string, error) {
	driver, err := r.drivers.For(ctx, c.Channel)
	if err != nil {
		return "", fmt.Errorf("channel %q: %w", c.Channel, err)
	}
	return driver.Post(ctx, c, m)
}
