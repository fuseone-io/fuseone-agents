package connectortools

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

const maxCloudflareResponseBytes = 1 << 20

// maxCloudflareListPages bounds pagination. The list a firewall rule reads
// stays small by design — the daily ceiling sees to it — so a list that
// pages past this is not the list this connector manages.
const maxCloudflareListPages = 20

// cloudflareRemoteError carries the status and nothing else: the remote body
// may hold anything, and an error string reaches logs and run records.
type cloudflareRemoteError struct{ status int }

func (e cloudflareRemoteError) Error() string {
	return fmt.Sprintf("connector: Cloudflare returned status %d", e.status)
}

// CloudflareListItem is the fixed projection of one list entry. Nothing else
// from the remote payload is kept.
type CloudflareListItem struct {
	ID        string `json:"id"`
	IP        string `json:"ip"`
	Comment   string `json:"comment"`
	CreatedOn string `json:"created_on"`
}

type HTTPCloudflareClient struct {
	client *http.Client
}

func NewHTTPCloudflareClient(client *http.Client) *HTTPCloudflareClient {
	if client == nil {
		client = guardedHTTPClient()
	}
	clone := *client
	clone.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("connector: Cloudflare redirects are refused")
	}
	if clone.Timeout == 0 || clone.Timeout > 30*time.Second {
		clone.Timeout = 30 * time.Second
	}
	return &HTTPCloudflareClient{client: &clone}
}

func cloudflareItemsURL(cfg CloudflareConfig, cursor string) string {
	u := fmt.Sprintf("%s/client/v4/accounts/%s/rules/lists/%s/items",
		cloudflareBaseURL(cfg), cfg.AccountID, cfg.ListID)
	if cursor != "" {
		u += "?cursor=" + cursor
	}
	return u
}

// Items reads the whole configured list through its cursor pagination.
func (c *HTTPCloudflareClient) Items(
	ctx context.Context, cfg CloudflareConfig, credential SecretValue,
) ([]CloudflareListItem, error) {
	var items []CloudflareListItem
	cursor := ""
	for range maxCloudflareListPages {
		page, next, err := c.itemsPage(ctx, cfg, credential, cursor)
		if err != nil {
			return nil, err
		}
		items = append(items, page...)
		if next == "" {
			return items, nil
		}
		cursor = next
	}
	return nil, fmt.Errorf("connector: the Cloudflare list pages past the managed size")
}

func (c *HTTPCloudflareClient) itemsPage(
	ctx context.Context, cfg CloudflareConfig, credential SecretValue, cursor string,
) ([]CloudflareListItem, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cloudflareItemsURL(cfg, cursor), nil)
	if err != nil {
		return nil, "", fmt.Errorf("connector: Cloudflare request: %w", err)
	}
	body, err := c.do(req, credential)
	if err != nil {
		return nil, "", err
	}
	var decoded struct {
		Result     []CloudflareListItem `json:"result"`
		Success    bool                 `json:"success"`
		ResultInfo struct {
			Cursors struct {
				After string `json:"after"`
			} `json:"cursors"`
		} `json:"result_info"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil || !decoded.Success {
		return nil, "", fmt.Errorf("connector: Cloudflare returned an unusable list page")
	}
	return decoded.Result, decoded.ResultInfo.Cursors.After, nil
}

// AddItem appends one entry. Cloudflare applies list writes asynchronously
// and answers with an operation id; the entry appears within seconds, and
// the id is returned for the run's record.
func (c *HTTPCloudflareClient) AddItem(
	ctx context.Context, cfg CloudflareConfig, credential SecretValue, ip, comment string,
) (string, error) {
	payload, err := json.Marshal([]map[string]string{{"ip": ip, "comment": comment}})
	if err != nil {
		return "", fmt.Errorf("connector: encode Cloudflare item: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cloudflareItemsURL(cfg, ""), bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("connector: Cloudflare request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	body, err := c.do(req, credential)
	if err != nil {
		return "", err
	}
	var decoded struct {
		Result struct {
			OperationID string `json:"operation_id"`
		} `json:"result"`
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil || !decoded.Success {
		return "", fmt.Errorf("connector: Cloudflare refused the list write")
	}
	return decoded.Result.OperationID, nil
}

// do sends with the bearer and returns the limited body. Errors never carry
// the credential or the remote body: a status is a status.
func (c *HTTPCloudflareClient) do(req *http.Request, credential SecretValue) ([]byte, error) {
	if credential.empty() {
		return nil, fmt.Errorf("connector: the Cloudflare instance has no token")
	}
	req.Header.Set("Authorization", "Bearer "+credential.value)
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("connector: Cloudflare request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
		return nil, cloudflareRemoteError{status: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCloudflareResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("connector: Cloudflare response unreadable")
	}
	return body, nil
}
