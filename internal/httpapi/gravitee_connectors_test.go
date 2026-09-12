package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/settings"
)

func graviteeBody() *openapi.ConnectorGraviteeInput {
	return &openapi.ConnectorGraviteeInput{
		Address:      "https://apim.example/management/v2",
		Organization: "org-prod", Environment: "env-prod",
		AllowedReferences: []openapi.ConnectorGraviteeReference{{Type: openapi.API, Id: "checkout-api"}},
		MinTTLSeconds:     86400, MaxTTLSeconds: 90 * 86400,
		CredentialSource: openapi.ConnectorGraviteeCredentialSource{
			Kind:          openapi.ConnectorGraviteeCredentialSourceKindVaultKvSecret,
			VaultInstance: "secrets", Path: "integrations/gravitee/prod", Field: "access_token",
		},
	}
}

func TestPutConnectorInstance_carriesTheGraviteeBoundary(t *testing.T) {
	t.Parallel()

	spy := &connectorInstanceSpy{}
	resp, err := NewServer(ledger.NewMemory(), "test").WithConnectorInstances(spy).
		PutConnectorInstance(as(domain.RoleCurator), openapi.PutConnectorInstanceRequestObject{
			Name: "apim",
			Body: &openapi.PutConnectorInstanceJSONRequestBody{
				Connector: "gravitee", ScopeKind: openapi.ConnectorScopeKindArea,
				Company: ptr("acme"), Area: ptr("platform"), Gravitee: graviteeBody(),
			},
		})
	if err != nil {
		t.Fatalf("PutConnectorInstance: %v", err)
	}
	if _, ok := resp.(openapi.PutConnectorInstance204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	cfg := spy.put.Gravitee
	if cfg.Organization != "org-prod" || cfg.Environment != "env-prod" ||
		len(cfg.AllowedReferences) != 1 || cfg.AllowedReferences[0].ID != "checkout-api" {
		t.Fatalf("Gravitee = %+v, want the fixed remote boundary", cfg)
	}
	if cfg.CredentialSource.Path != "integrations/gravitee/prod" ||
		cfg.CredentialSource.Field != "access_token" {
		t.Fatalf("credential source = %+v, want the fixed Vault location", cfg.CredentialSource)
	}
}

func TestConnectorInstanceResponses_hideTheVaultLocationFromTheOrdinaryListing(t *testing.T) {
	t.Parallel()

	cfg := connectortools.GraviteeConfig{
		Address:      "https://apim.example/management/v2",
		Organization: "org-prod", Environment: "env-prod",
		AllowedReferences: []connectortools.GraviteeReference{{
			Type: connectortools.GraviteeReferenceAPI, ID: "checkout-api",
		}},
		MinTTLSeconds: 86400, MaxTTLSeconds: 90 * 86400,
		CredentialSource: connectortools.GraviteeCredentialSource{
			Kind:          connectortools.GraviteeCredentialVaultKV,
			VaultInstance: "secrets", Path: "SECRET-PATH-CANARY", Field: "SECRET-FIELD-CANARY",
		},
	}
	configured := connectortools.ConfiguredInstance{
		Instance: connectortools.Instance{
			Name: "apim", Connector: "gravitee", Enabled: true,
			Scope: domain.Scope{Company: "acme", Area: "platform"}, Gravitee: cfg,
		},
		ScopeKind: settings.ScopeArea,
	}
	listed, err := json.Marshal(connectorInstanceResponse(configured))
	if err != nil {
		t.Fatalf("marshal listing: %v", err)
	}
	if strings.Contains(string(listed), "SECRET-PATH-CANARY") ||
		strings.Contains(string(listed), "SECRET-FIELD-CANARY") {
		t.Fatalf("ordinary listing exposed the Vault location: %s", listed)
	}
	detail := connectorInstanceDetailResponse(configured)
	if detail.Gravitee == nil || detail.Gravitee.CredentialSource.Path != "SECRET-PATH-CANARY" ||
		detail.Gravitee.CredentialSource.Field != "SECRET-FIELD-CANARY" {
		t.Fatalf("configurer detail = %+v, want the authored Vault location", detail.Gravitee)
	}
}
