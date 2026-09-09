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
	at, err := r.PostPlaced(ctx, c, m)
	return at.Ref, err
}

// PostPlaced posts and answers with where the message landed, when the driver
// can say. One that cannot is taken at its word that the address is the place.
func (r *Router) PostPlaced(
	ctx context.Context, c Conversation, m Message,
) (Placement, error) {
	driver, err := r.drivers.For(ctx, c.Channel)
	if err != nil {
		return Placement{}, fmt.Errorf("channel %q: %w", c.Channel, err)
	}
	return placed(ctx, driver, c, m)
}

/*
RouterEditor rewrites messages on the connection that posted them.

A Closer holds one thing that edits, and a message names the connection it
belongs to rather than carrying its driver — so this pairs the two, resolving
per edit for the same reason the Router resolves per post: a token rotated is a
token the next call should use.
*/
type RouterEditor struct{ drivers Drivers }

func NewRouterEditor(d Drivers) *RouterEditor { return &RouterEditor{drivers: d} }

// Edit rewrites one message, on the connection the placement names.
//
// A driver with no way to rewrite what it posted is not a failure to retry:
// the card stays as it is, which is what every card does today.
func (e *RouterEditor) Edit(ctx context.Context, at Placement, m Message) error {
	driver, err := e.drivers.For(ctx, at.Channel)
	if err != nil {
		return fmt.Errorf("channel %q: %w", at.Channel, err)
	}
	editor, ok := driver.(Editors)
	if !ok {
		return NewError(CodeUnsupportedCapability,
			fmt.Sprintf("channel %q cannot rewrite a message it posted", at.Channel))
	}
	return editor.Edit(ctx, at, m)
}
