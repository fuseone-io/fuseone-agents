package connectortools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/netguard"
	"github.com/fuseone/agents/internal/settings"
)

type Settings struct {
	store *settings.Store
}

func NewSettings(store *settings.Store) *Settings { return &Settings{store: store} }

type ConfiguredInstance struct {
	Instance
	ScopeKind settings.ScopeKind
	HasToken  bool
	UpdatedBy string
	UpdatedAt time.Time
}

func (s *Settings) Configured(ctx context.Context) ([]ConfiguredInstance, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	rows, err := s.store.List(ctx, settings.KindConnectorInstance)
	if err != nil {
		return nil, err
	}
	out := make([]ConfiguredInstance, 0, len(rows))
	for _, row := range rows {
		instance, err := SettingInstance(row)
		if err != nil {
			return nil, err
		}
		out = append(out, ConfiguredInstance{
			Instance:  instance,
			ScopeKind: row.ScopeKind,
			HasToken:  row.HasSecret,
			UpdatedBy: row.UpdatedBy,
			UpdatedAt: row.UpdatedAt,
		})
	}
	return out, nil
}

func (s *Settings) Instances(ctx context.Context) ([]Instance, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	rows, err := s.store.List(ctx, settings.KindConnectorInstance)
	if err != nil {
		return nil, err
	}
	out := make([]Instance, 0, len(rows))
	for _, row := range rows {
		instance, err := s.instance(ctx, row)
		if err != nil {
			return nil, err
		}
		if instance.Enabled {
			out = append(out, instance)
		}
	}
	return out, nil
}

func (s *Settings) ToolEntries(ctx context.Context) ([]domain.ToolEntry, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	rows, err := s.store.List(ctx, settings.KindConnectorInstance)
	if err != nil {
		return nil, err
	}
	var instances []Instance
	for _, row := range rows {
		instance, err := SettingInstance(row)
		if err != nil {
			return nil, err
		}
		if !row.Enabled || (RequiresToken(instance.Connector) && !row.HasSecret) {
			continue
		}
		if err := ValidateInstanceConfig(instance); err != nil {
			return nil, err
		}
		instances = append(instances, instance)
	}
	return toolEntriesFor(instances), nil
}

type StoredInstance struct {
	Connector string      `json:"connector"`
	Vault     VaultConfig `json:"vault,omitempty"`
	// A pointer, because omitempty does not elide a struct: every vault
	// instance would otherwise store an empty sql object in a value operators
	// read.
	SQL *SQLConfig `json:"sql,omitempty"`
}

func (s *Settings) instance(ctx context.Context, row settings.Setting) (Instance, error) {
	instance, err := SettingInstance(row)
	if err != nil {
		return Instance{}, err
	}
	if !instance.Enabled {
		return instance, nil
	}
	revealed, err := s.store.Reveal(ctx, row.ScopeKind, row.Scope, settings.KindConnectorInstance, row.Name)
	if err != nil {
		return Instance{}, err
	}
	instance.Token = revealed.Secret
	return instance, ValidateInstance(instance)
}

func SettingInstance(row settings.Setting) (Instance, error) {
	var stored StoredInstance
	if err := json.Unmarshal(row.Value, &stored); err != nil {
		return Instance{}, fmt.Errorf("connector: decode %s: %w", row.Name, err)
	}
	return Instance{
		Connector: strings.TrimSpace(stored.Connector),
		Name:      row.Name,
		Scope:     instanceScope(row),
		Enabled:   row.Enabled,
		Vault:     stored.Vault,
		SQL:       storedSQL(stored.SQL),
		HasToken:  row.HasSecret,
	}, nil
}

func SettingValue(instance Instance) (json.RawMessage, error) {
	value, err := json.Marshal(StoredInstance{
		Connector: strings.TrimSpace(instance.Connector),
		Vault:     instance.Vault,
		SQL:       sqlToStore(instance.SQL),
	})
	if err != nil {
		return nil, fmt.Errorf("connector: encode %s: %w", instance.Name, err)
	}
	return value, nil
}

func instanceScope(row settings.Setting) domain.Scope {
	if row.ScopeKind == settings.ScopeInstallation {
		return domain.Scope{Company: domain.Installation}
	}
	return row.Scope
}

func ValidateInstance(instance Instance) error {
	if err := ValidateInstanceConfig(instance); err != nil {
		return err
	}
	// Asked per connector. Requiring one everywhere would either block SQL,
	// which has no token of its own, or invite somebody to satisfy the check
	// with the database password this connector exists to avoid storing.
	if !RequiresToken(instance.Connector) {
		if strings.TrimSpace(instance.Token) != "" {
			return fmt.Errorf("connector: %s %s must not carry a token; its authority comes from its binding",
				instance.Connector, instance.Name)
		}
		return nil
	}
	if strings.TrimSpace(instance.Token) == "" {
		return fmt.Errorf("connector: %s %s needs a token", instance.Connector, instance.Name)
	}
	return nil
}

func ValidateInstanceConfig(instance Instance) error {
	if !ValidInstanceName(instance.Name) {
		return fmt.Errorf("connector: invalid instance name %q", instance.Name)
	}
	switch instance.Connector {
	case "vault":
		return validateVaultConfig(instance)
	case "sql":
		return validateSQLConfig(instance)
	default:
		return fmt.Errorf("connector: unsupported connector %q", instance.Connector)
	}
}

func validateVaultConfig(instance Instance) error {
	switch {
	case strings.TrimSpace(instance.Vault.Address) == "":
		return fmt.Errorf("connector: vault %s needs an address", instance.Name)
	case strings.TrimSpace(instance.Vault.Mount) == "":
		return fmt.Errorf("connector: vault %s needs a mount", instance.Name)
	case len(instance.Vault.AllowedPathPrefixes) == 0:
		return fmt.Errorf("connector: vault %s needs allowed path prefixes", instance.Name)
	}
	if err := netguard.ValidateHTTPURL(instance.Vault.Address); err != nil {
		if errors.Is(err, netguard.ErrBlockedAddress) {
			return fmt.Errorf("connector: vault %s address cannot target cloud metadata or link-local networks", instance.Name)
		}
		return fmt.Errorf("connector: vault %s address must be http or https", instance.Name)
	}
	for _, prefix := range instance.Vault.AllowedPathPrefixes {
		if cleanVaultPath(prefix) == "" {
			return fmt.Errorf("connector: vault %s has an invalid path prefix", instance.Name)
		}
	}
	return nil
}

// storedSQL and sqlToStore keep Instance.SQL a plain value with a useful zero,
// while the stored shape stays absent for connectors that have no SQL config.
func storedSQL(cfg *SQLConfig) SQLConfig {
	if cfg == nil {
		return SQLConfig{}
	}
	return *cfg
}

func sqlToStore(cfg SQLConfig) *SQLConfig {
	if cfg == (SQLConfig{}) {
		return nil
	}
	return &cfg
}

/*
ConfiguredInstances is the non-secret read: every instance, no token.

The credential resolver uses this rather than Instances, which reveals the
token of every enabled instance to answer any question. Choosing a vault does
not require holding the credentials of all of them.
*/
func (s *Settings) ConfiguredInstances(ctx context.Context) ([]ConfiguredInstance, error) {
	return s.Configured(ctx)
}

/*
RevealVaultToken reveals exactly one token, for an instance already resolved.

Named so that Instances cannot satisfy the interface by accident: the resolver
asks for the token of the vault it chose, after choosing, and the store reads
that one setting.
*/
func (s *Settings) RevealVaultToken(
	ctx context.Context, instance ConfiguredInstance,
) (string, error) {
	if s == nil || s.store == nil {
		return "", fmt.Errorf("connector: no settings store")
	}
	_, storedScope, err := settingScope(instance.ScopeKind, instance.Scope)
	if err != nil {
		return "", err
	}
	revealed, err := s.store.Reveal(
		ctx, instance.ScopeKind, storedScope, settings.KindConnectorInstance, instance.Name)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(revealed.Secret) == "" {
		return "", fmt.Errorf("connector: vault %s has no stored token", instance.Name)
	}
	return revealed.Secret, nil
}

// settingScope is the scope a setting is stored under, which is not always the
// scope it runs in: an installation-wide row stores an empty scope and runs as
// the installation company.
func settingScope(kind settings.ScopeKind, scope domain.Scope) (domain.Scope, domain.Scope, error) {
	switch kind {
	case settings.ScopeInstallation:
		return domain.Scope{Company: domain.Installation}, domain.Scope{}, nil
	case settings.ScopeCompany:
		return domain.Scope{Company: scope.Company}, domain.Scope{Company: scope.Company}, nil
	case settings.ScopeArea:
		return scope, scope, nil
	}
	return domain.Scope{}, domain.Scope{}, fmt.Errorf("connector: unknown scope kind %q", kind)
}
