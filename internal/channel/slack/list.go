package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

/*
Which conversations this bot can be pointed at.

`users.conversations` rather than `conversations.list`, and the difference is
the whole point: it answers with the conversations the bot is already a member
of. A picker built on it cannot offer a channel where posting would fail, so
the commonest mistake in configuring this — forgetting to invite the bot —
stops being possible rather than being caught later by a notification that
never arrived.

The name is offered and the identifier is stored. A Slack channel can be
renamed and its id cannot, so storing what the operator recognised would break
delivery on the day somebody tidied up the workspace.
*/

// Conversation is a place the bot can post, as a person would recognise it.
type Conversation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Private is shown so an operator can tell two similarly named channels
	// apart, which is the moment they are most likely to pick the wrong one.
	Private bool `json:"is_private"`
	// Archived is asked for in the query and checked again here. Slack has
	// been inconsistent about honouring exclude_archived for private
	// conversations, and an archived channel offered in a picker is a
	// configuration that saves cleanly and delivers nothing.
	Archived bool `json:"is_archived"`
}

// pages is a ceiling on how far the walk goes.
//
// A thousand conversations is far past what a bot is invited to, and a cursor
// that never empties would otherwise hold a request open until it timed out.
const pages = 10

func (p *Poster) Conversations(ctx context.Context) ([]Conversation, error) {
	var (
		out    []Conversation
		cursor string
	)
	for range pages {
		page, next, err := p.conversationPage(ctx, cursor)
		if err != nil {
			return nil, err
		}
		for _, c := range page {
			if c.Archived {
				continue
			}
			out = append(out, c)
		}
		if next == "" {
			return out, nil
		}
		cursor = next
	}
	return out, nil
}

func (p *Poster) conversationPage(
	ctx context.Context, cursor string,
) ([]Conversation, string, error) {
	query := url.Values{
		// Both, because an alerts channel is as often private as public.
		"types": {"public_channel,private_channel"},
		// An archived channel accepts no messages, and offering one is
		// offering a configuration that silently delivers nothing.
		"exclude_archived": {"true"},
		"limit":            {"200"},
	}
	if cursor != "" {
		query.Set("cursor", cursor)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.base+"/users.conversations?"+query.Encode(), nil)
	if err != nil {
		return nil, "", fmt.Errorf("slack: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.token)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("slack: list conversations: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var answer struct {
		OK       bool           `json:"ok"`
		Channels []Conversation `json:"channels"`
		Error    string         `json:"error"`
		// Needed names the scope Slack wants. An app granted only chat:write
		// can post and cannot list, and an empty answer there would read as
		// "the bot is in no channels" — which sends somebody to fix the wrong
		// thing entirely.
		Needed   string `json:"needed"`
		Metadata struct {
			NextCursor string `json:"next_cursor"`
		} `json:"response_metadata"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		return nil, "", fmt.Errorf("slack: read answer (status %d): %w", resp.StatusCode, err)
	}
	if !answer.OK {
		if answer.Needed != "" {
			return nil, "", fmt.Errorf("slack: refused: %s (the app needs %s)",
				answer.Error, answer.Needed)
		}
		return nil, "", fmt.Errorf("slack: refused: %s", answer.Error)
	}
	return answer.Channels, answer.Metadata.NextCursor, nil
}
