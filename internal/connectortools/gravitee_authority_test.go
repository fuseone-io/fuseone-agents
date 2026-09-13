package connectortools

import "testing"

func TestValidGraviteeAuthority_rechecksRestoredConfiguration(t *testing.T) {
	t.Parallel()
	runScope := area("acme", "platform")
	gravitee := ConfiguredInstance{Instance: graviteeInstance(runScope, graviteeSource("secrets"))}
	vault := ConfiguredInstance{Instance: vaultInstance("secrets", runScope)}
	vault.Vault.AllowedPathPrefixes = []string{"gravitee/prod"}
	gravitee.Gravitee.CredentialSource.Path = "gravitee/prod/token"

	if !validGraviteeAuthority(gravitee, vault) {
		t.Fatal("valid authority was refused")
	}

	tests := []struct {
		name   string
		change func(*ConfiguredInstance, *ConfiguredInstance)
	}{
		{"path outside the Vault allowlist", func(g, _ *ConfiguredInstance) {
			g.Gravitee.CredentialSource.Path = "another/team/token"
		}},
		{"invalid restored Gravitee URL", func(g, _ *ConfiguredInstance) {
			g.Gravitee.Address = "http://gravitee.example.com"
		}},
		{"invalid restored Vault URL", func(_, v *ConfiguredInstance) {
			v.Vault.Address = "://broken"
		}},
		{"Vault that does not cover the Gravitee scope", func(_, v *ConfiguredInstance) {
			v.Scope = area("acme", "security")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, v := gravitee, vault
			tt.change(&g, &v)
			if validGraviteeAuthority(g, v) {
				t.Fatal("restored invalid authority was accepted")
			}
		})
	}
}
