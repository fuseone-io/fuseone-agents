package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var (
	errGitHub  = errors.New("github")
	errNoToken = errors.New("GITHUB_TOKEN is not set")
	errNotRead = errors.New("factory-measure only reads")
)

// client is a read-only GitHub client: GraphQL queries and REST GETs, never
// a mutation, never another method.
type client struct {
	http       *http.Client
	token      string
	graphqlURL string
	apiURL     string
}

func newClient(getenv func(string) string, hc *http.Client) (*client, error) {
	token := strings.TrimSpace(getenv("GITHUB_TOKEN"))
	if token == "" {
		return nil, errNoToken
	}
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	return &client{
		http:       hc,
		token:      token,
		graphqlURL: orDefault(getenv("GITHUB_GRAPHQL_URL"), "https://api.github.com/graphql"),
		apiURL:     strings.TrimRight(orDefault(getenv("GITHUB_API_URL"), "https://api.github.com"), "/"),
	}, nil
}

func orDefault(v, def string) string {
	if v = strings.TrimSpace(v); v != "" {
		return v
	}
	return def
}

type gqlError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// query runs a GraphQL query and decodes its data into out. A document that
// is not a query is refused before anything is sent.
func (c *client) query(ctx context.Context, doc string, vars map[string]any, out any) error {
	if !strings.HasPrefix(strings.TrimSpace(doc), "query") {
		return errNotRead
	}
	body, err := json.Marshal(map[string]any{"query": doc, "variables": vars})
	if err != nil {
		return fmt.Errorf("%w: encode query: %w", errGitHub, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.graphqlURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: %w", errGitHub, err)
	}
	var res struct {
		Data   json.RawMessage `json:"data"`
		Errors []gqlError      `json:"errors"`
	}
	if _, err := c.do(req, &res); err != nil {
		return err
	}
	if len(res.Errors) > 0 {
		return fmt.Errorf("%w: graphql: %s", errGitHub, describe(res.Errors))
	}
	return json.Unmarshal(res.Data, out)
}

func describe(errs []gqlError) string {
	var parts []string
	for _, e := range errs {
		parts = append(parts, strings.TrimSpace(e.Type+" "+e.Message))
	}
	return strings.Join(parts, "; ")
}

// getPages GETs a REST list and every page its Link header names, calling
// each with every page's body. A next page on another host is refused: the
// token goes only to the API it was given for.
func (c *client) getPages(ctx context.Context, path string, each func(json.RawMessage) error) error {
	next := c.apiURL + path
	for next != "" {
		if !strings.HasPrefix(next, c.apiURL+"/") {
			return fmt.Errorf("%w: a next page outside %s", errGitHub, c.apiURL)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return fmt.Errorf("%w: %w", errGitHub, err)
		}
		var page json.RawMessage
		h, err := c.do(req, &page)
		if err != nil {
			return err
		}
		if err := each(page); err != nil {
			return err
		}
		next = nextLink(h.Get("Link"))
	}
	return nil
}

func (c *client) do(req *http.Request, out any) (http.Header, error) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s %s: %w", errGitHub, req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: %s %s: %w", errGitHub, req.Method, req.URL.Path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s %s: HTTP %d", errGitHub, req.Method, req.URL.Path, resp.StatusCode)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, fmt.Errorf("%w: %s %s: %w", errGitHub, req.Method, req.URL.Path, err)
	}
	return resp.Header, nil
}

var linkNextRE = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func nextLink(h string) string {
	if m := linkNextRE.FindStringSubmatch(h); m != nil {
		return m[1]
	}
	return ""
}
