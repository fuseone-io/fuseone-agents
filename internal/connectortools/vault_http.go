package connectortools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/netguard"
)

type HTTPVaultClient struct {
	client *http.Client
}

func NewHTTPVaultClient(client *http.Client) *HTTPVaultClient {
	if client == nil {
		client = guardedHTTPClient()
	}
	return &HTTPVaultClient{client: client}
}

func (c *HTTPVaultClient) WriteSecret(
	ctx context.Context, cfg VaultConfig, token, secretPath string, fields map[string]VaultSecretField,
) (VaultWriteResult, error) {
	payload := map[string]any{"data": vaultData(fields)}
	var out struct {
		Data struct {
			Version int `json:"version"`
		} `json:"data"`
	}
	if err := c.do(ctx, cfg, token, http.MethodPost, vaultKVPath(cfg, "data", secretPath), payload, &out); err != nil {
		return VaultWriteResult{}, err
	}
	return VaultWriteResult{Version: out.Data.Version}, nil
}

func (c *HTTPVaultClient) ReadMetadata(
	ctx context.Context, cfg VaultConfig, token, secretPath string,
) (VaultMetadata, error) {
	var out vaultMetadataResponseWire
	if err := c.do(ctx, cfg, token, http.MethodGet, vaultKVPath(cfg, "metadata", secretPath), nil, &out); err != nil {
		return VaultMetadata{}, err
	}
	return out.metadata(), nil
}

func (c *HTTPVaultClient) RevokeLease(ctx context.Context, cfg VaultConfig, token, leaseID string) error {
	body := map[string]string{"lease_id": leaseID}
	return c.do(ctx, cfg, token, http.MethodPut, "sys/leases/revoke", body, nil)
}

func (c *HTTPVaultClient) do(
	ctx context.Context, cfg VaultConfig, token, method, apiPath string, body, into any,
) error {
	req, err := vaultRequest(ctx, cfg, token, method, apiPath, body)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("vault: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
		return fmt.Errorf("vault: status %d", resp.StatusCode)
	}
	if into == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(into); err != nil {
		return fmt.Errorf("vault: decode response: %w", err)
	}
	return nil
}

func vaultRequest(
	ctx context.Context, cfg VaultConfig, token, method, apiPath string, body any,
) (*http.Request, error) {
	endpoint, err := vaultURL(cfg, apiPath)
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("vault: encode request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, fmt.Errorf("vault: build request: %w", err)
	}
	req.Header.Set("X-Vault-Token", token)
	if cfg.Namespace != "" {
		req.Header.Set("X-Vault-Namespace", cfg.Namespace)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func vaultURL(cfg VaultConfig, apiPath string) (string, error) {
	if err := netguard.ValidateHTTPURL(cfg.Address); err != nil {
		return "", err
	}
	base, err := url.Parse(strings.TrimRight(cfg.Address, "/") + "/")
	if err != nil {
		return "", fmt.Errorf("vault: parse address: %w", err)
	}
	base.Path = path.Join(base.Path, "v1", apiPath)
	return base.String(), nil
}

func guardedHTTPClient() *http.Client {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		transport = &http.Transport{}
	}
	transport = transport.Clone()
	netguard.GuardHTTPTransport(transport)
	return &http.Client{
		Transport: netguard.RefuseEnvironmentProxy(transport),
		Timeout:   30 * time.Second,
	}
}

func vaultKVPath(cfg VaultConfig, kind, secretPath string) string {
	return path.Join(strings.Trim(cfg.Mount, "/"), kind, secretPath)
}

func vaultData(fields map[string]VaultSecretField) map[string]any {
	out := make(map[string]any, len(fields))
	for name, field := range fields {
		out[name] = string(field.Value)
	}
	return out
}

type vaultMetadataResponseWire struct {
	Data struct {
		CurrentVersion int                                  `json:"current_version"`
		Versions       map[string]vaultMetadataVersionEntry `json:"versions"`
	} `json:"data"`
}

type vaultMetadataVersionEntry struct {
	Destroyed bool `json:"destroyed"`
}

func (r vaultMetadataResponseWire) metadata() VaultMetadata {
	versions := make([]int, 0, len(r.Data.Versions))
	for raw, entry := range r.Data.Versions {
		if entry.Destroyed {
			continue
		}
		version, err := strconv.Atoi(raw)
		if err == nil {
			versions = append(versions, version)
		}
	}
	slices.Sort(versions)
	return VaultMetadata{CurrentVersion: r.Data.CurrentVersion, Versions: versions}
}

/*
IssueDatabaseCredential asks the database secrets engine for one short-lived
credential.

Not among the operations a model can reach. It exists for the SQL runtime,
after Gate and approval, and the fields it returns never become tool output.

The response is read into a narrow shape rather than a map: a decoder that
accepted anything would carry whatever else Vault returned into memory, and
from there into whatever printed it.
*/
func (c *HTTPVaultClient) IssueDatabaseCredential(
	ctx context.Context, cfg VaultConfig, token, mount, role string,
) (VaultDatabaseCredential, error) {
	apiPath, err := vaultCredentialPath(mount, role)
	if err != nil {
		return VaultDatabaseCredential{}, err
	}
	var body struct {
		LeaseID       string `json:"lease_id"`
		LeaseDuration int    `json:"lease_duration"`
		Data          struct {
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"data"`
	}
	// do discards a non-2xx body and reports only the status, which is the
	// guarantee this needed: a Vault error body can quote the path, the role
	// and the policy that refused.
	if err := c.do(ctx, cfg, token, http.MethodGet, apiPath, nil, &body); err != nil {
		return VaultDatabaseCredential{}, err
	}
	return VaultDatabaseCredential{
		Username: body.Data.Username, Password: body.Data.Password,
		LeaseID: body.LeaseID, LeaseTTLSeconds: body.LeaseDuration,
	}, nil
}

/*
vaultCredentialPath builds the endpoint from parts that were checked first.

The mount is a relative path with no segment that climbs, and the role is a
single strict segment. A binding is a field an operator edits by hand, and
`database/../sys` is a plausible typo that would address something else
entirely — so the shape is refused rather than cleaned up, because cleaning it
would silently address a third thing.
*/
func vaultCredentialPath(mount, role string) (string, error) {
	cleanMount, err := relativeVaultPath(mount)
	if err != nil {
		return "", fmt.Errorf("vault: invalid database mount: %w", err)
	}
	if !vaultSegment.MatchString(role) {
		return "", fmt.Errorf("vault: invalid database role")
	}
	return cleanMount + "/creds/" + role, nil
}

func relativeVaultPath(mount string) (string, error) {
	if mount == "" || strings.HasPrefix(mount, "/") {
		return "", fmt.Errorf("a mount is a relative path")
	}
	segments := strings.Split(mount, "/")
	for _, segment := range segments {
		if !vaultSegment.MatchString(segment) {
			return "", fmt.Errorf("a mount segment is not a plain name")
		}
	}
	return strings.Join(segments, "/"), nil
}

// vaultSegment is one path segment and nothing that could end it: no slash, no
// dot-dot, no whitespace, no control byte. Named positively so the next
// character somebody's proxy treats as a separator is refused by default.
var vaultSegment = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
