package slack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/fuseone/agents/internal/channel"
)

/*
Account reads one Slack account's profile with users.info.

Needs `users:read`, and `users:read.email` for the email: without that scope
Slack answers ok and leaves the email out, so an empty Email means "not
shared", never "has none". A deactivated account or a bot is refused rather
than described: neither is somebody asking for anything.
*/
func (p *Poster) Account(ctx context.Context, account string) (channel.Account, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.base+"/users.info?"+url.Values{"user": {account}}.Encode(), nil)
	if err != nil {
		return channel.Account{}, fmt.Errorf("slack: build account request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.token)

	resp, err := p.client.Do(req)
	if err != nil {
		return channel.Account{}, fmt.Errorf("slack: read account: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var answer struct {
		OK   bool `json:"ok"`
		User struct {
			Name    string `json:"name"`
			Deleted bool   `json:"deleted"`
			IsBot   bool   `json:"is_bot"`
			Profile struct {
				RealName    string `json:"real_name"`
				DisplayName string `json:"display_name"`
				Email       string `json:"email"`
			} `json:"profile"`
		} `json:"user"`
		Error  string `json:"error"`
		Needed string `json:"needed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		return channel.Account{}, fmt.Errorf("slack: read account answer (status %d): %w", resp.StatusCode, err)
	}
	switch {
	case !answer.OK && answer.Needed != "":
		return channel.Account{}, fmt.Errorf("slack: refused: %s (the app needs %s)", answer.Error, answer.Needed)
	case !answer.OK:
		return channel.Account{}, fmt.Errorf("slack: refused: %s", answer.Error)
	case answer.User.Deleted || answer.User.IsBot:
		return channel.Account{}, errors.New("slack: the account is deactivated or a bot")
	}
	profile := answer.User.Profile
	return channel.Account{
		Name:  firstNonEmpty(profile.RealName, profile.DisplayName, answer.User.Name),
		Email: strings.TrimSpace(profile.Email),
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
