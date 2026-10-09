package connectortools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const maxESResponseBytes = 1 << 20

// esRemoteError carries the status and nothing else: a search error body
// can quote the request and the documents, and an error string reaches
// logs and run records.
type esRemoteError struct{ status int }

func (e esRemoteError) Error() string {
	return fmt.Sprintf("connector: Elasticsearch returned status %d", e.status)
}

type HTTPElasticsearchClient struct {
	client *http.Client
}

func NewHTTPElasticsearchClient(client *http.Client) *HTTPElasticsearchClient {
	if client == nil {
		client = guardedHTTPClient()
	}
	clone := *client
	clone.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("connector: Elasticsearch redirects are refused")
	}
	if clone.Timeout == 0 || clone.Timeout > 30*time.Second {
		clone.Timeout = 30 * time.Second
	}
	return &HTTPElasticsearchClient{client: &clone}
}

// Search posts one body to one index's _search. The index travels as a URL
// path segment and is configuration-validated upstream; it is escaped here
// anyway, because defence against a crafted segment belongs at the edge
// that builds the URL.
func (c *HTTPElasticsearchClient) Search(
	ctx context.Context, cfg ElasticsearchConfig, credential SecretValue,
	index string, body []byte,
) ([]byte, error) {
	if credential.empty() {
		return nil, fmt.Errorf("connector: the Elasticsearch instance has no password")
	}
	endpoint := fmt.Sprintf("%s/%s/_search",
		trimRightSlash(cfg.BaseURL), url.PathEscape(index))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("connector: Elasticsearch request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(cfg.Username, credential.value)
	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("connector: Elasticsearch request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
		return nil, esRemoteError{status: resp.StatusCode}
	}
	out, err := io.ReadAll(io.LimitReader(resp.Body, maxESResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("connector: Elasticsearch response unreadable")
	}
	return out, nil
}

func trimRightSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
