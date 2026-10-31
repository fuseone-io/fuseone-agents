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

// ErrNotSent marks a request that failed before it left the process: no
// token, or no request could be built. For a write, it means the write is
// known not to have happened.
var ErrNotSent = errors.New("github request not sent")

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

// apiRequest is one REST call. Body and Out are `any` because they are what
// encoding/json takes: a request struct to encode, a pointer to decode into.
type apiRequest struct {
	Method, Path string
	Body, Out    any
}

// build prepares a request without sending it. Every failure here is
// ErrNotSent: nothing has left the process.
func (c *Client) build(ctx context.Context, r apiRequest) (*http.Request, error) {
	token, err := c.opts.Tokens.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: github token: %w", ErrNotSent, err)
	}
	var reader io.Reader
	if r.Body != nil {
		raw, err := json.Marshal(r.Body)
		if err != nil {
			return nil, fmt.Errorf("%w: github %s %s: encode: %w", ErrNotSent, r.Method, r.Path, err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, c.opts.APIURL+r.Path, reader)
	if err != nil {
		return nil, fmt.Errorf("%w: github %s %s: %w", ErrNotSent, r.Method, r.Path, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: github %s %s: %w", ErrNotSent, r.Method, r.Path, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return req, nil
}

// send performs one request and returns the status code. Out is decoded only
// on a 2xx; any other status is left to the caller to interpret. A failure
// before the request was handed to the transport wraps ErrNotSent; any later
// one (transport error, timeout, an undecodable 2xx) does not, because the
// request may have reached GitHub.
func (c *Client) send(ctx context.Context, r apiRequest) (int, error) {
	method, path, out := r.Method, r.Path, r.Out
	req, err := c.build(ctx, r)
	if err != nil {
		return 0, err
	}
	// From here the request may reach GitHub: any failure is not ErrNotSent.
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
// out is `any`: a pointer for encoding/json to decode into.
func (c *Client) get(ctx context.Context, path string, out any) error {
	code, err := c.send(ctx, apiRequest{Method: http.MethodGet, Path: path, Out: out})
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
