package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/fuseone/agents/internal/channel"
)

/*
OpenReview starts the thread a ticket is worked in.

The room is where the answer is written and corrected before the person who
asked sees anything, so the opening message says two things and no more: that
there is a request, and where it came from. The link is Slack's own permalink
rather than one assembled here — a workspace has a domain this package was
never told, and a wrong link in the first message of every ticket is the kind
of wrong that nobody reports and everybody works around.
*/
func (p *Poster) OpenReview(
	ctx context.Context, opening channel.ReviewOpening,
) (string, error) {
	text := ":eyes: *A new ticket arrived* — I am looking at it.\n" +
		p.ticketLink(ctx, opening) +
		"\nI will come back in this thread when I need you."
	no := false
	return p.send(ctx, postMessage{
		Channel:     opening.Conversation,
		Text:        text,
		Parse:       "none",
		UnfurlLinks: &no,
		UnfurlMedia: &no,
	})
}

// ticketLink answers the line that points at the request. A permalink that
// cannot be read is not a reason to leave the room without its thread: the
// conversation and the message are said plainly instead, and both are enough
// to find the request by hand.
func (p *Poster) ticketLink(ctx context.Context, opening channel.ReviewOpening) string {
	plain := fmt.Sprintf(":thread: <#%s> · %s", escape(opening.From), escape(opening.Root))
	query := url.Values{"channel": {opening.From}, "message_ts": {opening.Root}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.base+"/chat.getPermalink?"+query.Encode(), nil)
	if err != nil {
		return plain
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	resp, err := p.client.Do(req)
	if err != nil {
		return plain
	}
	defer func() { _ = resp.Body.Close() }()
	var answer struct {
		OK        bool   `json:"ok"`
		Permalink string `json:"permalink"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil ||
		!answer.OK || answer.Permalink == "" {
		return plain
	}
	return ":thread: <" + escape(answer.Permalink) + "|The ticket thread>"
}
