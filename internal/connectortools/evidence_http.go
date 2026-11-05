package connectortools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	// maxEvidencePageBytes bounds one response. Past it the page is an error,
	// because a prefix of a page is fewer events reported as if they were all.
	maxEvidencePageBytes = 64 << 20
	// evidenceRequestTimeout is generous on purpose: event stores answer a
	// large page in minutes, not seconds.
	evidenceRequestTimeout = 2 * time.Minute
)

// EvidenceRequest is one GET against the source.
type EvidenceRequest struct {
	Path  string
	Query url.Values
}

// evidenceRemoteError carries the status and nothing else: a source's error
// body can quote the query, and an error string reaches logs and run records.
type evidenceRemoteError struct{ status int }

func (e evidenceRemoteError) Error() string {
	return fmt.Sprintf("connector: evidence source returned status %d", e.status)
}

type HTTPEvidenceClient struct {
	client *http.Client
}

func NewHTTPEvidenceClient(client *http.Client) *HTTPEvidenceClient {
	if client == nil {
		client = guardedHTTPClient()
	}
	clone := *client
	// The token must not follow a redirect to wherever the response points.
	clone.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("connector: evidence source redirects are refused")
	}
	clone.Timeout = evidenceRequestTimeout
	return &HTTPEvidenceClient{client: &clone}
}

// Fetch asks for one page. The URL is built only from configuration and values
// the layer has already validated.
func (c *HTTPEvidenceClient) Fetch(
	ctx context.Context, cfg EvidenceConfig, credential SecretValue, request EvidenceRequest,
) ([]byte, error) {
	if credential.empty() {
		return nil, errors.New("connector: the evidence instance has no token")
	}
	endpoint := trimRightSlash(cfg.BaseURL) + request.Path
	if encoded := request.Query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		// The URL can hold personal data from a filter, so it is not quoted.
		return nil, errors.New("connector: evidence request could not be built")
	}
	req.Header.Set(cfg.AuthHeader, cfg.AuthPrefix+credential.value)
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, errors.New("connector: evidence request failed")
	}
	defer resp.Body.Close()
	return readEvidencePage(resp)
}

func readEvidencePage(resp *http.Response) ([]byte, error) {
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
		return nil, evidenceRemoteError{status: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxEvidencePageBytes+1))
	if err != nil {
		return nil, errors.New("connector: evidence response unreadable")
	}
	if len(body) > maxEvidencePageBytes {
		return nil, errors.New("connector: evidence page exceeds the limit")
	}
	return body, nil
}
