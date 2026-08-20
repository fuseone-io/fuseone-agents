package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/fuseone/agents/internal/channel"
)

const (
	threadContextMessages = 20
	threadContextBytes    = 32 * 1024
	threadMessageBytes    = 4 * 1024
)

/*
Thread reads the messages that came before an ask in a Slack thread.

The ask itself is excluded with `latest` and `inclusive=false`: the point is to
give the agent the alert somebody replied to, not to echo the instruction it
already received as the ask text. Bounded because this text enters a model
prompt, and a long incident thread should not turn one mention into an
unlimited context dump.
*/
func (p *Poster) Thread(
	ctx context.Context, conversation, thread, before string,
) (channel.ThreadContext, error) {
	query := url.Values{
		"channel":   {conversation},
		"ts":        {thread},
		"limit":     {fmt.Sprint(threadContextMessages)},
		"inclusive": {"false"},
	}
	if before != "" {
		query.Set("latest", before)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.base+"/conversations.replies?"+query.Encode(), nil)
	if err != nil {
		return channel.ThreadContext{}, fmt.Errorf("slack: build thread request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.token)

	resp, err := p.client.Do(req)
	if err != nil {
		return channel.ThreadContext{}, fmt.Errorf("slack: read thread: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var answer struct {
		OK       bool `json:"ok"`
		Messages []struct {
			TS    string `json:"ts"`
			User  string `json:"user"`
			BotID string `json:"bot_id"`
			AppID string `json:"app_id"`
			Text  string `json:"text"`
		} `json:"messages"`
		HasMore bool   `json:"has_more"`
		Error   string `json:"error"`
		Needed  string `json:"needed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		return channel.ThreadContext{}, fmt.Errorf("slack: read thread answer (status %d): %w", resp.StatusCode, err)
	}
	if !answer.OK {
		if answer.Needed != "" {
			return channel.ThreadContext{}, fmt.Errorf("slack: refused: %s (the app needs %s)",
				answer.Error, answer.Needed)
		}
		return channel.ThreadContext{}, fmt.Errorf("slack: refused: %s", answer.Error)
	}

	out := channel.ThreadContext{
		Conversation: conversation,
		Thread:       thread,
		Truncated:    answer.HasMore,
	}
	used := 0
	for _, m := range answer.Messages {
		text := strings.TrimSpace(m.Text)
		if text == "" || m.TS == before {
			continue
		}
		limited, cut := trimThreadText(text, threadMessageBytes)
		if cut {
			out.Truncated = true
		}
		if used+len(limited) > threadContextBytes {
			out.Truncated = true
			break
		}
		used += len(limited)
		out.Messages = append(out.Messages, channel.ThreadMessage{
			Ref:    m.TS,
			Source: channel.Source{User: m.User, Bot: m.BotID, App: m.AppID}.Key(),
			Text:   limited,
		})
	}
	return out, nil
}

func trimThreadText(text string, max int) (string, bool) {
	if len(text) <= max {
		return text, false
	}
	cut := 0
	for i := range text {
		if i > max {
			break
		}
		cut = i
	}
	if cut == 0 {
		cut = max
	}
	return strings.TrimSpace(text[:cut]) + " [truncated]", true
}
