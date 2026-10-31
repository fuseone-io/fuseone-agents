package githubapp

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
Why a mint failed, as a stable reason somebody can act on.

The response body is never quoted. GitHub's own messages are harmless, but a
proxy or a misconfigured base URL can answer with anything, and the request
that provoked it carried the App's JWT. A status and a fixed sentence are
enough to tell a revoked key from a removed installation, which are the two
fixes an operator has to choose between.
*/
var (
	ErrAppRefused = errors.New(
		"github app: GitHub refused the App's JWT — the private key or App id is wrong, or the key was revoked")
	ErrInstallationNotFound = errors.New(
		"github app: installation not found — the App was uninstalled or the installation id is wrong")
	ErrMintForbidden = errors.New(
		"github app: the installation refused the token request — a repository is outside what it was granted")
	ErrMintFailed = errors.New("github app: the installation token request failed")
)

// token is a minted installation access token and when it stops working.
type token struct {
	value     string
	expiresAt time.Time
}

type minter struct {
	client *http.Client
	app    domain.MCPGitHubApp
	key    *rsa.PrivateKey
}

// mint exchanges a fresh App JWT for an installation access token.
func (m minter) mint(ctx context.Context, now time.Time) (token, error) {
	jwt, err := appJWT(m.key, m.app.AppID, now)
	if err != nil {
		return token{}, err
	}
	req, err := m.request(ctx, jwt)
	if err != nil {
		return token{}, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return token{}, fmt.Errorf("%w: %w", ErrMintFailed, err)
	}
	defer resp.Body.Close()
	if err := statusError(resp.StatusCode); err != nil {
		return token{}, err
	}
	return readToken(resp.Body)
}

func (m minter) request(ctx context.Context, jwt string) (*http.Request, error) {
	var body io.Reader = http.NoBody
	if len(m.app.Repositories) > 0 {
		raw, err := json.Marshal(map[string][]string{"repositories": m.app.Repositories})
		if err != nil {
			return nil, fmt.Errorf("github app: encode repositories: %w", err)
		}
		body = bytes.NewReader(raw)
	}
	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", m.app.APIBase(), m.app.InstallationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %w", ErrMintFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != http.NoBody {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func statusError(status int) error {
	switch {
	case status == http.StatusCreated || status == http.StatusOK:
		return nil
	case status == http.StatusUnauthorized:
		return fmt.Errorf("%w (HTTP %d)", ErrAppRefused, status)
	case status == http.StatusNotFound:
		return fmt.Errorf("%w (HTTP %d)", ErrInstallationNotFound, status)
	case status == http.StatusForbidden || status == http.StatusUnprocessableEntity:
		return fmt.Errorf("%w (HTTP %d)", ErrMintForbidden, status)
	default:
		return fmt.Errorf("%w (HTTP %d)", ErrMintFailed, status)
	}
}

func readToken(body io.Reader) (token, error) {
	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 1<<20)).Decode(&out); err != nil {
		return token{}, fmt.Errorf("%w: unreadable response", ErrMintFailed)
	}
	if out.Token == "" || out.ExpiresAt.IsZero() {
		return token{}, fmt.Errorf("%w: response omitted token or expires_at", ErrMintFailed)
	}
	return token{value: out.Token, expiresAt: out.ExpiresAt}, nil
}
