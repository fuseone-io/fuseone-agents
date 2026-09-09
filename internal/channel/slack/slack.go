/*
Package slack posts what a run has to say into a Slack conversation.

Two things here are not the HTTP.

Slack answers 200 with `{"ok": false}` for most of what actually goes wrong —
a bot removed from a channel, a revoked token, a conversation that no longer
exists. A driver that trusts the status code reports success for every one of
them, and the platform learns it is being ignored by never hearing anything
again.

And the message is composed here rather than by the caller. Blocks are Slack's
answer to "a heading, some facts and a link"; Teams answers with an Adaptive
Card and SMS with neither. A caller that knew one of those formats would end up
knowing all of them.
*/
package slack

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/channel"
)

// API is Slack's own base. Overridable for tests and for an installation
// behind a proxy that terminates outbound traffic.
const API = "https://slack.com/api"

// Poster posts as one bot, to whichever conversation it is asked for.
type Poster struct {
	token string
	base  string
	// decidable is whether this connection can verify what comes back. The
	// driver is the right thing to know it: it holds the secret that would do
	// the verifying, and a button on a channel that cannot check an answer
	// would promise a surface that is switched off.
	decidable bool
	client    *http.Client
}

func New(token string) *Poster {
	return &Poster{
		token: token,
		base:  API,
		// Bounded, because a channel that hangs must not hold the sweep that
		// still has other conversations to reach.
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Decidable says this connection can verify what comes back, which is what
// lets its messages carry buttons.
func (p *Poster) Decidable() *Poster {
	p.decidable = true
	return p
}

// WithEndpointBase points every call at another host.
func (p *Poster) WithEndpointBase(url string) *Poster {
	p.base = strings.TrimSuffix(url, "/")
	return p
}

// Post sends one message and answers with the timestamp a reply threads on.
func (p *Poster) Post(
	ctx context.Context, c channel.Conversation, m channel.Message,
) (string, error) {
	at, err := p.PostPlaced(ctx, c, m)
	return at.Ref, err
}

/*
PostPlaced posts and says where the message landed.

An address and a place are the same string for a room and different for a
person: chat.postMessage accepts a user id and opens the conversation itself,
answering with the one it made. Editing that message later takes the place, and
nothing but this call ever learns it.
*/
func (p *Poster) PostPlaced(
	ctx context.Context, c channel.Conversation, m channel.Message,
) (channel.Placement, error) {
	return p.call(ctx, "/chat.postMessage", postMessage{
		Channel: c.ID,
		// Fallback text as well as blocks. Notifications and screen readers
		// read this one, so a message with blocks alone is silent on a phone.
		Text:   summary(m),
		Blocks: blocks(m, p.decidable),
	})
}

/*
Edit replaces a message this platform posted.

How a card stops offering an answer to a question that already has one. It is
not a decision and cannot become one: it rewrites what is on screen, touches no
state, and its worst failure is a button that answers 409 — which is what
happens today, everywhere.
*/
func (p *Poster) Edit(
	ctx context.Context, at channel.Placement, m channel.Message,
) error {
	_, err := p.call(ctx, "/chat.update", postMessage{
		Channel: at.Conversation, Ts: at.Ref,
		Text:   summary(m),
		Blocks: blocks(m, p.decidable),
	})
	return err
}

type postMessage struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
	Blocks  []any  `json:"blocks,omitempty"`
	Thread  string `json:"thread_ts,omitempty"`
	// Ts names the message being replaced. Only chat.update reads it.
	Ts          string `json:"ts,omitempty"`
	Parse       string `json:"parse,omitempty"`
	Mrkdwn      *bool  `json:"mrkdwn,omitempty"`
	UnfurlLinks *bool  `json:"unfurl_links,omitempty"`
	UnfurlMedia *bool  `json:"unfurl_media,omitempty"`
}
