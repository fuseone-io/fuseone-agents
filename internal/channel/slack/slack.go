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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/fuseone/agents/internal/channel"
)

// Endpoint is Slack's own. Overridable for tests and for an installation
// behind a proxy that terminates outbound traffic.
const Endpoint = "https://slack.com/api/chat.postMessage"

// Poster posts as one bot, to whichever conversation it is asked for.
type Poster struct {
	token    string
	endpoint string
	client   *http.Client
}

func New(token string) *Poster {
	return &Poster{
		token:    token,
		endpoint: Endpoint,
		// Bounded, because a channel that hangs must not hold the sweep that
		// still has other conversations to reach.
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *Poster) WithEndpoint(url string) *Poster {
	p.endpoint = url
	return p
}

// Post sends one message and answers with the timestamp a reply threads on.
func (p *Poster) Post(
	ctx context.Context, c channel.Conversation, m channel.Message,
) (string, error) {
	body, err := json.Marshal(postMessage{
		Channel: c.ID,
		// Fallback text as well as blocks. Notifications and screen readers
		// read this one, so a message with blocks alone is silent on a phone.
		Text:   summary(m),
		Blocks: blocks(m),
	})
	if err != nil {
		return "", fmt.Errorf("slack: build message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("slack: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("slack: post: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var answer struct {
		OK    bool   `json:"ok"`
		TS    string `json:"ts"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		return "", fmt.Errorf("slack: read answer (status %d): %w", resp.StatusCode, err)
	}
	if !answer.OK {
		// Slack's own word for it. "not_in_channel" tells an operator what to
		// do; "post failed" tells them to go and find out.
		return "", fmt.Errorf("slack: refused: %s", answer.Error)
	}
	return answer.TS, nil
}

type postMessage struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
	Blocks  []any  `json:"blocks,omitempty"`
	Thread  string `json:"thread_ts,omitempty"`
}
