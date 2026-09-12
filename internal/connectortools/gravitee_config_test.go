package connectortools

import (
	"reflect"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

func graviteeVault(name string, scope domain.Scope) Instance {
	vault := vaultInstance(name, scope)
	vault.Vault.Mount = "secret"
	vault.Vault.AllowedPathPrefixes = []string{"integrations/gravitee"}
	return vault
}

func graviteeSource(vault string) GraviteeCredentialSource {
	return GraviteeCredentialSource{
		Kind: GraviteeCredentialVaultKV, VaultInstance: vault,
		Path: "integrations/gravitee/prod", Field: "access_token",
	}
}

func graviteeInstance(scope domain.Scope, source GraviteeCredentialSource) Instance {
	return Instance{
		Connector: "gravitee", Name: "apim", Scope: scope, Enabled: true,
		Gravitee: GraviteeConfig{
			Address:      "https://apim.internal/management/v2",
			Organization: "org-prod", Environment: "env-prod",
			AllowedReferences: []GraviteeReference{{Type: GraviteeReferenceAPI, ID: "checkout-api"}},
			MinTTLSeconds:     24 * 60 * 60, MaxTTLSeconds: 90 * 24 * 60 * 60,
			CredentialSource: source,
		},
	}
}

func TestValidateInstanceConfig_graviteeAcceptsOnlyABoundedAPIScope(t *testing.T) {
	t.Parallel()

	if err := ValidateInstanceConfig(graviteeInstance(
		area("acme", "platform"), graviteeSource("secrets"))); err != nil {
		t.Fatalf("valid Gravitee config was refused: %v", err)
	}

	cases := map[string]func(*GraviteeConfig){
		"missing organization":   func(c *GraviteeConfig) { c.Organization = "" },
		"environment as a path":  func(c *GraviteeConfig) { c.Environment = "prod/../admin" },
		"unsupported reference":  func(c *GraviteeConfig) { c.AllowedReferences[0].Type = "API_PRODUCT" },
		"reference id as a path": func(c *GraviteeConfig) { c.AllowedReferences[0].ID = "../another-api" },
		"no allowed API":         func(c *GraviteeConfig) { c.AllowedReferences = nil },
		"second allowed API": func(c *GraviteeConfig) {
			c.AllowedReferences = append(c.AllowedReferences,
				GraviteeReference{Type: GraviteeReferenceAPI, ID: "payments-api"})
		},
		"credential path traversal": func(c *GraviteeConfig) { c.CredentialSource.Path = "integrations/gravitee/../admin" },
		"credential field as path":  func(c *GraviteeConfig) { c.CredentialSource.Field = "data/token" },
	}
	for name, breakConfig := range cases {
		instance := graviteeInstance(area("acme", "platform"), graviteeSource("secrets"))
		breakConfig(&instance.Gravitee)
		if err := ValidateInstanceConfig(instance); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestValidateInstanceConfig_graviteeOwnsHTTPSAndTTLPolicy(t *testing.T) {
	t.Parallel()

	badAddresses := []string{
		"http://apim.internal/management/v2",
		"https://user:secret@apim.internal/management/v2",
		"https://apim.internal/management/v2?token=secret",
		"https://apim.internal/management/v2#fragment",
		"https://169.254.169.254/management/v2",
	}
	for _, address := range badAddresses {
		instance := graviteeInstance(area("acme", "platform"), graviteeSource("secrets"))
		instance.Gravitee.Address = address
		if err := ValidateInstanceConfig(instance); err == nil {
			t.Errorf("address %q was accepted", address)
		}
	}

	cases := map[string]func(*GraviteeConfig){
		"no minimum":        func(c *GraviteeConfig) { c.MinTTLSeconds = 0 },
		"no maximum":        func(c *GraviteeConfig) { c.MaxTTLSeconds = 0 },
		"inverted range":    func(c *GraviteeConfig) { c.MinTTLSeconds = c.MaxTTLSeconds + 1 },
		"unbounded maximum": func(c *GraviteeConfig) { c.MaxTTLSeconds = 366 * 24 * 60 * 60 },
	}
	for name, breakConfig := range cases {
		instance := graviteeInstance(area("acme", "platform"), graviteeSource("secrets"))
		breakConfig(&instance.Gravitee)
		if err := ValidateInstanceConfig(instance); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestValidateInstance_graviteeRefusesACredentialOfItsOwn(t *testing.T) {
	t.Parallel()

	const marker = "GRAVITEE-TOKEN-CANARY"
	instance := graviteeInstance(area("acme", "platform"), graviteeSource("secrets"))
	instance.Token = marker
	err := ValidateInstance(instance)
	if err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("ValidateInstance err = %v, want a refusal naming the token field", err)
	}
	if strings.Contains(err.Error(), marker) {
		t.Fatalf("the refusal repeated the credential: %v", err)
	}
}

func TestValidateBindings_vaultInAWiderScopeMaySupplyGravitee(t *testing.T) {
	t.Parallel()

	err := ValidateBindings([]Instance{
		graviteeVault("secrets", company("acme")),
		graviteeInstance(area("acme", "platform"), graviteeSource("secrets")),
	})
	if err != nil {
		t.Fatalf("ValidateBindings err = %v, want the wider vault scope to cover", err)
	}
}

func TestValidateBindings_graviteeRefusesAnUnsafeOrAmbiguousVaultBinding(t *testing.T) {
	t.Parallel()

	runScope := area("acme", "platform")
	wrongScope := graviteeVault("secrets", area("acme", "payments"))
	disabled := graviteeVault("secrets", company("acme"))
	disabled.Enabled = false
	tokenless := graviteeVault("secrets", company("acme"))
	tokenless.Token = ""
	plain := graviteeVault("secrets", company("acme"))
	plain.Vault.Address = "http://vault.internal"
	wrongPath := graviteeVault("secrets", company("acme"))
	wrongPath.Vault.AllowedPathPrefixes = []string{"another/service"}
	other := Instance{Connector: "smtp", Name: "secrets", Scope: company("acme"), Enabled: true}

	cases := map[string][]Instance{
		"missing":     nil,
		"wrong scope": {wrongScope},
		"disabled":    {disabled},
		"tokenless":   {tokenless},
		"plain HTTP":  {plain},
		"wrong path":  {wrongPath},
		"not Vault":   {other},
		"ambiguous": {
			graviteeVault("secrets", company("acme")),
			graviteeVault("secrets", runScope),
		},
	}
	for name, dependencies := range cases {
		instances := append([]Instance{}, dependencies...)
		instances = append(instances, graviteeInstance(runScope, graviteeSource("secrets")))
		if err := ValidateBindings(instances); err == nil {
			t.Errorf("%s binding was accepted", name)
		}
	}
}

func TestSettingValue_roundTripsTheGraviteeBindingWithoutASecret(t *testing.T) {
	t.Parallel()

	original := graviteeInstance(area("acme", "platform"), graviteeSource("secrets"))
	original.Token = "GRAVITEE-TOKEN-CANARY"
	value, err := SettingValue(original)
	if err != nil {
		t.Fatalf("SettingValue: %v", err)
	}
	if strings.Contains(string(value), original.Token) {
		t.Fatalf("stored value carries the token: %s", value)
	}
	back, err := SettingInstance(settings.Setting{
		Name: original.Name, Value: value, Enabled: true,
		ScopeKind: settings.ScopeArea, Scope: original.Scope,
	})
	if err != nil {
		t.Fatalf("SettingInstance: %v", err)
	}
	if !reflect.DeepEqual(back.Gravitee, original.Gravitee) {
		t.Fatalf("Gravitee = %+v, want %+v", back.Gravitee, original.Gravitee)
	}

	vaultValue, err := SettingValue(graviteeVault("secrets", company("acme")))
	if err != nil {
		t.Fatalf("SettingValue(vault): %v", err)
	}
	if strings.Contains(string(vaultValue), `"gravitee"`) {
		t.Fatalf("a vault persisted an empty Gravitee object: %s", vaultValue)
	}
}

func TestToolEntries_plannedGraviteeOffersNothingToTheModel(t *testing.T) {
	t.Parallel()

	entries := toolEntriesFor([]Instance{
		graviteeInstance(area("acme", "platform"), graviteeSource("secrets")),
	})
	for _, entry := range entries {
		if strings.HasPrefix(string(entry.ID), "gravitee.") {
			t.Fatalf("planned Gravitee operation reached the model: %s", entry.ID)
		}
	}
}
