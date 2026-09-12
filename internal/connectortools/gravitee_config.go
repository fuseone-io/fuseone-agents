package connectortools

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/fuseone/agents/internal/netguard"
)

type GraviteeReferenceType string
type GraviteeCredentialSourceKind string

const (
	GraviteeReferenceAPI GraviteeReferenceType = "API"

	GraviteeCredentialVaultKV GraviteeCredentialSourceKind = "vault_kv_secret"
	maxGraviteeReferences                                  = 100
	maxGraviteeTTLSeconds                                  = 365 * 24 * 60 * 60
)

// GraviteeReference is a remote resource the configured instance may reach.
// It is configuration, never a tool argument.
type GraviteeReference struct {
	Type GraviteeReferenceType
	ID   string
}

// GraviteeCredentialSource names one secret already constrained by a Vault
// connector. The worker reads the field internally; no model tool can ask for
// the path, field or value.
type GraviteeCredentialSource struct {
	Kind          GraviteeCredentialSourceKind
	VaultInstance string
	Path          string
	Field         string
}

type GraviteeConfig struct {
	Address           string
	Organization      string
	Environment       string
	AllowedReferences []GraviteeReference
	MinTTLSeconds     int
	MaxTTLSeconds     int
	AllowNoExpiry     bool
	CredentialSource  GraviteeCredentialSource
}

var graviteeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func validateGraviteeConfig(instance Instance) error {
	cfg := instance.Gravitee
	if err := validateGraviteeAddress(instance.Name, cfg.Address); err != nil {
		return err
	}
	switch {
	case !graviteeID.MatchString(cfg.Organization):
		return fmt.Errorf("connector: gravitee %s needs a valid organization id", instance.Name)
	case !graviteeID.MatchString(cfg.Environment):
		return fmt.Errorf("connector: gravitee %s needs a valid environment id", instance.Name)
	case cfg.MinTTLSeconds <= 0:
		return fmt.Errorf("connector: gravitee %s needs a positive minimum TTL", instance.Name)
	case cfg.MaxTTLSeconds < cfg.MinTTLSeconds:
		return fmt.Errorf("connector: gravitee %s maximum TTL must not be below its minimum", instance.Name)
	case cfg.MaxTTLSeconds > maxGraviteeTTLSeconds:
		return fmt.Errorf("connector: gravitee %s maximum TTL exceeds one year", instance.Name)
	}
	if err := validateGraviteeReferences(instance.Name, cfg.AllowedReferences); err != nil {
		return err
	}
	return validateGraviteeCredentialSource(instance.Name, cfg.CredentialSource)
}

func validateGraviteeAddress(name, raw string) error {
	if err := netguard.ValidateHTTPURL(raw); err != nil {
		if errors.Is(err, netguard.ErrBlockedAddress) {
			return fmt.Errorf("connector: gravitee %s address cannot target cloud metadata or link-local networks", name)
		}
		return fmt.Errorf("connector: gravitee %s needs an https address", name)
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("connector: gravitee %s needs an https address without credentials, query or fragment", name)
	}
	decoded, err := url.PathUnescape(u.EscapedPath())
	if err != nil || strings.Contains(decoded, "\x00") || hasRelativeSegment(decoded) {
		return fmt.Errorf("connector: gravitee %s address has an invalid path", name)
	}
	return nil
}

func hasRelativeSegment(p string) bool {
	for _, segment := range strings.Split(p, "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

func validateGraviteeReferences(name string, references []GraviteeReference) error {
	if len(references) == 0 || len(references) > maxGraviteeReferences {
		return fmt.Errorf("connector: gravitee %s needs between 1 and %d allowed API references",
			name, maxGraviteeReferences)
	}
	seen := make(map[string]bool, len(references))
	for _, reference := range references {
		if reference.Type != GraviteeReferenceAPI || !graviteeID.MatchString(reference.ID) {
			return fmt.Errorf("connector: gravitee %s has an invalid API reference", name)
		}
		key := string(reference.Type) + "\x00" + reference.ID
		if seen[key] {
			return fmt.Errorf("connector: gravitee %s repeats an allowed API reference", name)
		}
		seen[key] = true
	}
	return nil
}

func validateGraviteeCredentialSource(name string, source GraviteeCredentialSource) error {
	switch {
	case source.Kind != GraviteeCredentialVaultKV:
		return fmt.Errorf("connector: gravitee %s needs a credential source of kind %q",
			name, GraviteeCredentialVaultKV)
	case !ValidInstanceName(source.VaultInstance):
		return fmt.Errorf("connector: gravitee %s credential source needs a valid vault instance", name)
	case !strictVaultPath(source.Path):
		return fmt.Errorf("connector: gravitee %s credential source has an invalid path", name)
	case !vaultFieldRE.MatchString(source.Field):
		return fmt.Errorf("connector: gravitee %s credential source has an invalid field", name)
	}
	return nil
}

func strictVaultPath(raw string) bool {
	if raw == "" || raw != strings.TrimSpace(raw) || len(raw) > 512 ||
		strings.HasPrefix(raw, "/") || strings.HasSuffix(raw, "/") {
		return false
	}
	for _, segment := range strings.Split(raw, "/") {
		if segment == "." || segment == ".." || !vaultFieldRE.MatchString(segment) {
			return false
		}
	}
	return true
}
