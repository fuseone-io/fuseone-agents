package connectortools

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"strings"

	"github.com/fuseone/agents/internal/netguard"
)

// defaultCloudflareBaseURL is the public API. An instance may point elsewhere
// — a mock in a lab, a gateway — under the same address rules as every
// connector.
const defaultCloudflareBaseURL = "https://api.cloudflare.com"

// defaultCloudflareMaxBlocksPerDay bounds how many addresses the block list
// may grow by in one day when the instance does not say. The ceiling exists
// so a misled run cannot empty the internet into the list; an operator who
// needs more raises it knowingly.
const defaultCloudflareMaxBlocksPerDay = 20

// CloudflareConfig configures one block-list instance. Everything that is
// specific to an installation — which ranges must never be blocked, how many
// blocks a day are plausible — is configuration here, never a constant: this
// connector ships generic.
type CloudflareConfig struct {
	// BaseURL is the API endpoint; empty means the public API.
	BaseURL   string
	AccountID string
	// ListID names the account-level IP list a firewall rule references.
	// The connector edits that list and nothing else.
	ListID string
	// ProtectedRanges are CIDRs block_ip refuses on top of the private and
	// reserved ranges the code always refuses: the installation's own egress,
	// an anonymizer range whose addresses are shared by many real clients.
	ProtectedRanges []string
	// MaxBlocksPerDay caps the automatic entries added per calendar day,
	// counted from the list itself. Zero means the default.
	MaxBlocksPerDay int
}

var cloudflareID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func validateCloudflareConfig(instance Instance) error {
	cfg := instance.Cloudflare
	if cfg.BaseURL != "" {
		if err := validateCloudflareBaseURL(instance.Name, cfg.BaseURL); err != nil {
			return err
		}
	}
	switch {
	case !cloudflareID.MatchString(cfg.AccountID):
		return fmt.Errorf("connector: cloudflare %s needs a valid account id", instance.Name)
	case !cloudflareID.MatchString(cfg.ListID):
		return fmt.Errorf("connector: cloudflare %s needs a valid list id", instance.Name)
	case cfg.MaxBlocksPerDay < 0:
		return fmt.Errorf("connector: cloudflare %s daily cap cannot be negative", instance.Name)
	}
	for _, raw := range cfg.ProtectedRanges {
		// A bare address is refused on purpose: a protected set wants to be
		// explicit about its width, and "203.0.113.9" read as /32 by one
		// reader and as a typo'd /24 by another is how a hole opens.
		if _, err := netip.ParsePrefix(strings.TrimSpace(raw)); err != nil {
			return fmt.Errorf("connector: cloudflare %s protected range %q is not a CIDR", instance.Name, raw)
		}
	}
	return nil
}

func validateCloudflareBaseURL(name, raw string) error {
	if err := netguard.ValidateHTTPURL(raw); err != nil {
		if errors.Is(err, netguard.ErrBlockedAddress) {
			return fmt.Errorf("connector: cloudflare %s base url cannot target cloud metadata or link-local networks", name)
		}
		return fmt.Errorf("connector: cloudflare %s needs an https base url", name)
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("connector: cloudflare %s needs an https base url without credentials, query or fragment", name)
	}
	return nil
}

// cloudflareBaseURL is the configured endpoint or the public default.
func cloudflareBaseURL(cfg CloudflareConfig) string {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return defaultCloudflareBaseURL
	}
	return strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
}

// cloudflareDailyCap is the configured ceiling or the default.
func cloudflareDailyCap(cfg CloudflareConfig) int {
	if cfg.MaxBlocksPerDay <= 0 {
		return defaultCloudflareMaxBlocksPerDay
	}
	return cfg.MaxBlocksPerDay
}

func storedCloudflare(cfg *CloudflareConfig) CloudflareConfig {
	if cfg == nil {
		return CloudflareConfig{}
	}
	return *cfg
}

func cloudflareToStore(cfg CloudflareConfig) *CloudflareConfig {
	if cfg.BaseURL == "" && cfg.AccountID == "" && cfg.ListID == "" &&
		len(cfg.ProtectedRanges) == 0 && cfg.MaxBlocksPerDay == 0 {
		return nil
	}
	return &cfg
}
