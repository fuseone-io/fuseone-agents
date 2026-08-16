package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

/*
Saying a sentence back, and why it is escaped here.

A report is composed from a run's facts; a reply is a sentence somebody is owed
and it goes in the thread they asked in. Same connection, same bot, different
shape — so it shares the transport and nothing else.

What it does not share is trust. A refusal quotes what was typed, and Slack
reads `<!channel>` in message text as a broadcast rather than as characters:
the platform's own explanation would notify everybody in the conversation, with
the bot's permission rather than the asker's. Escaped at this edge for the
reason blocks are composed at this edge — it is the only place that knows what
this vendor treats as markup.
*/

// Say replies in a thread. Plain text, no blocks: there is nothing to decide,
// and a button that decides nothing is the worst thing to put on a refusal.
func (p *Poster) Say(ctx context.Context, conversation, thread, text string) error {
	_, err := p.send(ctx, postMessage{
		Channel: conversation,
		Thread:  thread,
		Text:    escape(text),
	})
	return err
}

/*
escape shows the characters instead of letting them act.

Slack's three, in the order the vendor documents: the ampersand first, or the
entities written by the other two would be escaped in turn and the reader would
see `&amp;lt;`.

This is not HTML escaping and Go's `html.EscapeString` is not a substitute: it
also rewrites quotes, and a refusal quoting what somebody typed is mostly
quotes.
*/
func escape(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	return strings.ReplaceAll(text, ">", "&gt;")
}

/*
send is the one round trip, and the one place `ok:false` is believed.

Slack answers 200 for most of what actually goes wrong — a bot removed from the
channel, a revoked token, a conversation that no longer exists. Two callers
reading the status code is two chances to report a message that never left, so
there is one reader.
*/
func (p *Poster) send(ctx context.Context, m postMessage) (string, error) {
	body, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("slack: build message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.base+"/chat.postMessage", bytes.NewReader(body))
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
