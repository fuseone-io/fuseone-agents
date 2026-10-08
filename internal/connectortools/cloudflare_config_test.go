package connectortools

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

func validCloudflareInstance() Instance {
	return Instance{
		Connector: "cloudflare",
		Name:      "edge",
		Enabled:   true,
		Cloudflare: CloudflareConfig{
			AccountID:       "0123456789abcdef0123456789abcdef",
			ListID:          "fedcba9876543210fedcba9876543210",
			ProtectedRanges: []string{"203.0.113.0/24", "2001:db8::/32"},
		},
	}
}

func TestValidateCloudflareConfig_acceptsTheMinimalInstance(t *testing.T) {
	t.Parallel()
	if err := ValidateInstanceConfig(validCloudflareInstance()); err != nil {
		t.Fatalf("ValidateInstanceConfig: %v", err)
	}
}

// The base URL is optional — the public API is the default — but once given it
// obeys the same rules as every connector address: https, no credentials, and
// never cloud metadata.
func TestValidateCloudflareConfig_refusesWhatMustBeRefused(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*Instance){
		"empty account id": func(i *Instance) { i.Cloudflare.AccountID = "" },
		"account id with spaces": func(i *Instance) {
			i.Cloudflare.AccountID = "not an id"
		},
		"empty list id": func(i *Instance) { i.Cloudflare.ListID = "" },
		"http base url": func(i *Instance) {
			i.Cloudflare.BaseURL = "http://api.cloudflare.com"
		},
		"metadata base url": func(i *Instance) {
			i.Cloudflare.BaseURL = "https://169.254.169.254/"
		},
		"base url with credentials": func(i *Instance) {
			i.Cloudflare.BaseURL = "https://user:pw@api.cloudflare.com"
		},
		"protected range that is not a cidr": func(i *Instance) {
			i.Cloudflare.ProtectedRanges = []string{"not-a-range"}
		},
		"protected range as a bare ip": func(i *Instance) {
			i.Cloudflare.ProtectedRanges = []string{"203.0.113.9"}
		},
		"negative daily cap": func(i *Instance) {
			i.Cloudflare.MaxBlocksPerDay = -1
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			instance := validCloudflareInstance()
			mutate(&instance)
			if err := ValidateInstanceConfig(instance); err == nil {
				t.Fatal("ValidateInstanceConfig accepted it")
			}
		})
	}
}

// Cloudflare authenticates with a token of its own, sealed in the settings
// row like the vault connector's — not a binding.
func TestValidateInstance_cloudflareNeedsItsOwnToken(t *testing.T) {
	t.Parallel()
	if !RequiresToken("cloudflare") {
		t.Fatal("RequiresToken(cloudflare) = false")
	}
	instance := validCloudflareInstance()
	if err := ValidateInstance(instance); err == nil {
		t.Fatal("an enabled cloudflare instance with no token was accepted")
	}
	instance.Token = "cf-token"
	if err := ValidateInstance(instance); err != nil {
		t.Fatalf("ValidateInstance with token: %v", err)
	}
}

// The stored shape survives the round trip, protected ranges included, and an
// instance of another connector stores no cloudflare object at all.
func TestSettingValue_cloudflareRoundTrip(t *testing.T) {
	t.Parallel()
	instance := validCloudflareInstance()
	value, err := SettingValue(instance)
	if err != nil {
		t.Fatalf("SettingValue: %v", err)
	}
	if strings.Contains(string(value), "CANARY") {
		t.Fatal("the stored value grew a token field")
	}
	back, err := SettingInstance(settings.Setting{
		Name: instance.Name, Value: value, Enabled: true,
	})
	if err != nil {
		t.Fatalf("SettingInstance: %v", err)
	}
	if back.Cloudflare.AccountID != instance.Cloudflare.AccountID ||
		back.Cloudflare.ListID != instance.Cloudflare.ListID ||
		len(back.Cloudflare.ProtectedRanges) != 2 {
		t.Fatalf("round trip lost configuration: %+v", back.Cloudflare)
	}

	other, err := SettingValue(Instance{Connector: "vault", Name: "v",
		Vault: VaultConfig{Address: "https://vault.example", Mount: "kv",
			AllowedPathPrefixes: []string{"apps"}}})
	if err != nil {
		t.Fatalf("SettingValue(vault): %v", err)
	}
	if strings.Contains(string(other), "cloudflare") {
		t.Fatalf("a vault instance stored a cloudflare object: %s", other)
	}
}

// The catalog offers exactly the two block-list operations. block_ip is a
// write — the entry is reversible, expiry included — and never less than
// that, because the Gate prices approval policies on the effect.
func TestToolEntries_runtimeCloudflareOffersOnlyTheBlockListOperations(t *testing.T) {
	t.Parallel()
	instance := validCloudflareInstance()
	instance.HasToken = true
	entries := toolEntriesFor([]Instance{instance})
	if len(entries) != 2 {
		t.Fatalf("entries = %d (%+v), want list_blocks and block_ip", len(entries), entries)
	}
	effects := map[string]domain.Effect{}
	for _, entry := range entries {
		effects[string(entry.ID)] = entry.Effect
	}
	if effects["cloudflare.edge.block_ip"] != domain.EffectWrite {
		t.Errorf("block_ip effect = %v, want write", effects["cloudflare.edge.block_ip"])
	}
	if effects["cloudflare.edge.list_blocks"] != domain.EffectRead {
		t.Errorf("list_blocks effect = %v, want read", effects["cloudflare.edge.list_blocks"])
	}
}
