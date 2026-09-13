package connectortools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

var ErrNoGraviteeAuthority = errors.New("connector: no Gravitee authority for this run")

type GraviteeAccess struct {
	Config         GraviteeConfig
	ContractDigest string
	credential     SecretValue
}

type GraviteeAccesses interface {
	Resolve(ctx context.Context, instance string, scope domain.Scope) (GraviteeAccess, error)
}

type SecretFieldReader interface {
	ReadSecretField(
		ctx context.Context, cfg VaultConfig, token, secretPath, field string,
	) (SecretValue, error)
}

type GraviteeAccessResolver struct {
	settings *Settings
	vault    SecretFieldReader
}

func NewGraviteeAccessResolver(settings *Settings, vault SecretFieldReader) *GraviteeAccessResolver {
	return &GraviteeAccessResolver{settings: settings, vault: vault}
}

func (r *GraviteeAccessResolver) Resolve(
	ctx context.Context, name string, scope domain.Scope,
) (GraviteeAccess, error) {
	if r == nil || r.settings == nil || r.vault == nil {
		return GraviteeAccess{}, ErrNoGraviteeAuthority
	}
	local, err := r.settings.graviteeSource(ctx, name, scope)
	if err != nil {
		return GraviteeAccess{}, authorityError(err)
	}
	credential, err := r.vault.ReadSecretField(ctx, local.vault, local.vaultToken,
		local.source.Path, local.source.Field)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return GraviteeAccess{}, err
		}
		return GraviteeAccess{}, ErrNoGraviteeAuthority
	}
	contract, ok := graviteeContractDigest(local.gravitee, local.vault,
		"gravitee.accept_subscription")
	if !ok {
		return GraviteeAccess{}, ErrNoGraviteeAuthority
	}
	return GraviteeAccess{
		Config: local.gravitee, ContractDigest: contract, credential: credential,
	}, nil
}

type graviteeLocalSource struct {
	gravitee   GraviteeConfig
	vault      VaultConfig
	vaultToken string
	source     GraviteeCredentialSource
}

func (s *Settings) graviteeSource(
	ctx context.Context, name string, scope domain.Scope,
) (graviteeLocalSource, error) {
	if s == nil || s.store == nil {
		return graviteeLocalSource{}, ErrNoGraviteeAuthority
	}
	var out graviteeLocalSource
	err := s.store.ReadSnapshot(ctx, func(db settings.DB) error {
		instances, err := s.configuredInstancesTx(ctx, db)
		if err != nil {
			return err
		}
		gravitee, err := oneInstance(instances, "gravitee", name, scope)
		if err != nil {
			return err
		}
		source := gravitee.Gravitee.CredentialSource
		vault, err := oneInstance(instances, "vault", source.VaultInstance, scope)
		if err != nil || !validGraviteeAuthority(gravitee, vault) {
			return ErrNoGraviteeAuthority
		}
		_, storedScope, err := settingScope(vault.ScopeKind, vault.Scope)
		if err != nil {
			return err
		}
		revealed, err := s.store.RevealTx(ctx, db, vault.ScopeKind, storedScope,
			settings.KindConnectorInstance, vault.Name)
		if err != nil || strings.TrimSpace(revealed.Secret) == "" {
			return ErrNoGraviteeAuthority
		}
		out = graviteeLocalSource{
			gravitee: gravitee.Gravitee, vault: vault.Vault,
			vaultToken: revealed.Secret, source: source,
		}
		return nil
	})
	return out, err
}

func validGraviteeAuthority(gravitee, vault ConfiguredInstance) bool {
	if ValidateInstanceConfig(gravitee.Instance) != nil ||
		ValidateInstanceConfig(vault.Instance) != nil ||
		!vault.Scope.Contains(gravitee.Scope) {
		return false
	}
	source := gravitee.Gravitee.CredentialSource
	_, allowed := allowedPath(vault.Vault.AllowedPathPrefixes, source.Path)
	return source.Kind == GraviteeCredentialVaultKV &&
		source.VaultInstance == vault.Name && allowed
}

func (s *Settings) configuredInstancesTx(
	ctx context.Context, db settings.DB,
) ([]ConfiguredInstance, error) {
	rows, err := s.store.ListTx(ctx, db, settings.KindConnectorInstance)
	if err != nil {
		return nil, err
	}
	out := make([]ConfiguredInstance, 0, len(rows))
	for _, row := range rows {
		instance, err := SettingInstance(row)
		if err != nil {
			return nil, err
		}
		out = append(out, ConfiguredInstance{Instance: instance, ScopeKind: row.ScopeKind,
			HasToken: row.HasSecret, UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt})
	}
	return out, nil
}

func authorityError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("%w", ErrNoGraviteeAuthority)
}
