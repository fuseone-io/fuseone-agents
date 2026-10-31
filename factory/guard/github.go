package guard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrGitHub marks a GitHub response the guard could not use. Its message
// carries the method, the path the guard built and the status code, never the
// response body: a body is free text, and the Merger must not read any.
var ErrGitHub = errors.New("github")

// TokenSource supplies the bearer for each request.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// ClientOptions configure a Client.
type ClientOptions struct {
	APIURL string
	Repo   Repo
	HTTP   *http.Client
	Tokens TokenSource
}

// Client is a deliberately small REST client: the endpoints the two tools need
// and nothing else, each decoding only the fields a decision reads.
type Client struct {
	opts ClientOptions
}

// NewClient builds a Client. Redirects are refused: a renamed or transferred
// repository answers with a redirect, and following it would act on a
// repository the configuration never named.
func NewClient(opts ClientOptions) *Client {
	base := opts.HTTP
	if base == nil {
		base = &http.Client{Timeout: 30 * time.Second}
	}
	noRedirect := *base
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	opts.HTTP = &noRedirect
	return &Client{opts: opts}
}

// repoPath prefixes a path with the configured repository.
func (c *Client) repoPath(format string, args ...any) string {
	return fmt.Sprintf("/repos/%s/%s", c.opts.Repo.Owner, c.opts.Repo.Name) + fmt.Sprintf(format, args...)
}

// send performs one request and returns the status code. out is decoded only
// on a 2xx; any other status is left to the caller to interpret.
func (c *Client) send(ctx context.Context, method, path string, body, out any) (int, error) {
	token, err := c.opts.Tokens.Token(ctx)
	if err != nil {
		return 0, fmt.Errorf("github token: %w", err)
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("github %s %s: encode: %w", method, path, err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.opts.APIURL+path, reader)
	if err != nil {
		return 0, fmt.Errorf("github %s %s: %w", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.opts.HTTP.Do(req)
	if err != nil {
		return 0, fmt.Errorf("github %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 || out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode, nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return resp.StatusCode, fmt.Errorf("%w: %s %s: decode: %w", ErrGitHub, method, path, err)
	}
	return resp.StatusCode, nil
}

// get reads one resource and treats anything but 200 as a failure.
func (c *Client) get(ctx context.Context, path string, out any) error {
	code, err := c.send(ctx, http.MethodGet, path, nil, out)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("%w: GET %s: status %d", ErrGitHub, path, code)
	}
	return nil
}

// maxPages bounds a listing. Past it the guard fails closed rather than
// deciding on a partial list.
const maxPages = 10

// listAll reads every page of a listing. unwrap extracts the items from one
// page's body, since some endpoints wrap their array in an object.
func listAll[T any](ctx context.Context, c *Client, path string, unwrap func(json.RawMessage) ([]T, error)) ([]T, error) {
	const perPage = 100
	var all []T
	for page := 1; page <= maxPages; page++ {
		var raw json.RawMessage
		if err := c.get(ctx, fmt.Sprintf("%s?per_page=%d&page=%d", path, perPage, page), &raw); err != nil {
			return nil, err
		}
		items, err := unwrap(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: GET %s: decode: %w", ErrGitHub, path, err)
		}
		all = append(all, items...)
		if len(items) < perPage {
			return all, nil
		}
	}
	return nil, fmt.Errorf("%w: GET %s: more than %d pages", ErrGitHub, path, maxPages)
}

// plainList unwraps an endpoint that returns a bare array.
func plainList[T any](raw json.RawMessage) ([]T, error) {
	var items []T
	err := json.Unmarshal(raw, &items)
	return items, err
}
