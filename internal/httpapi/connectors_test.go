package httpapi

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/settings"
)

type connectorInstanceSpy struct {
	items      []connectortools.ConfiguredInstance
	putBy      domain.UserID
	putKind    settings.ScopeKind
	putScope   domain.Scope
	put        connectortools.Instance
	putToken   *string
	clearToken bool
	deleted    string
}

func (s *connectorInstanceSpy) ConnectorInstances(context.Context) ([]connectortools.ConfiguredInstance, error) {
	return s.items, nil
}

func (s *connectorInstanceSpy) PutConnectorInstance(
	_ context.Context, by domain.UserID, _ domain.Scope,
	kind settings.ScopeKind, scope domain.Scope,
	instance connectortools.Instance, token *string, clearToken bool,
) error {
	s.putBy, s.putKind, s.putScope = by, kind, scope
	s.put, s.putToken, s.clearToken = instance, token, clearToken
	return nil
}

func (s *connectorInstanceSpy) DeleteConnectorInstance(
	_ context.Context, by domain.UserID, _ domain.Scope,
	kind settings.ScopeKind, scope domain.Scope, name string,
) error {
	s.putBy, s.putKind, s.putScope, s.deleted = by, kind, scope, name
	return nil
}

func TestListConnectorInstances_reportsTokenPresenceNotTokenMaterial(t *testing.T) {
	t.Parallel()

	spy := &connectorInstanceSpy{items: []connectortools.ConfiguredInstance{{
		Instance: connectortools.Instance{
			Name: "prod", Connector: "vault", Enabled: true,
			Scope: domain.Scope{Company: "acme", Area: "platform"},
			Vault: connectortools.VaultConfig{
				Address: "https://vault.example", Mount: "secret",
				AllowedPathPrefixes: []string{"certs"},
			},
		},
		ScopeKind: settings.ScopeArea, HasToken: true, UpdatedBy: "usr_ana",
	}}}
	resp, err := NewServer(ledger.NewMemory(), "test").WithConnectorInstances(spy).
		ListConnectorInstances(as(domain.RoleCurator), openapi.ListConnectorInstancesRequestObject{})
	if err != nil {
		t.Fatalf("ListConnectorInstances: %v", err)
	}
	body := resp.(openapi.ListConnectorInstances200JSONResponse)
	if len(body.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(body.Items))
	}
	item := body.Items[0]
	if !item.HasToken || item.Vault == nil || item.Vault.Address != "https://vault.example" {
		t.Fatalf("item = %+v, want non-secret config and token presence", item)
	}
}

func TestPutConnectorInstance_mapsScopeAndTokenPatch(t *testing.T) {
	t.Parallel()

	spy := &connectorInstanceSpy{}
	token := "vault-token"
	resp, err := NewServer(ledger.NewMemory(), "test").WithConnectorInstances(spy).
		PutConnectorInstance(as(domain.RoleCurator), openapi.PutConnectorInstanceRequestObject{
			Name: "prod",
			Body: &openapi.PutConnectorInstanceJSONRequestBody{
				Connector: "vault", ScopeKind: openapi.ConnectorScopeKindArea,
				Company: ptr("acme"), Area: ptr("platform"), Token: &token,
				Vault: &openapi.ConnectorVaultConfig{
					Address: "https://vault.example", Mount: "secret",
					AllowedPathPrefixes: []string{"certs"},
				},
			},
		})
	if err != nil {
		t.Fatalf("PutConnectorInstance: %v", err)
	}
	if _, ok := resp.(openapi.PutConnectorInstance204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	if spy.putBy != "usr_ana" || spy.putKind != settings.ScopeArea ||
		spy.putScope != (domain.Scope{Company: "acme", Area: "platform"}) {
		t.Fatalf("put by=%s kind=%s scope=%s", spy.putBy, spy.putKind, spy.putScope)
	}
	if spy.put.Name != "prod" || spy.put.Connector != "vault" ||
		spy.putToken == nil || *spy.putToken != token {
		t.Fatalf("put = %+v token=%v", spy.put, spy.putToken)
	}
}

func TestDeleteConnectorInstance_requiresTheConfiguredScopeKey(t *testing.T) {
	t.Parallel()

	spy := &connectorInstanceSpy{}
	resp, err := NewServer(ledger.NewMemory(), "test").WithConnectorInstances(spy).
		DeleteConnectorInstance(as(domain.RoleCurator), openapi.DeleteConnectorInstanceRequestObject{
			Name: "prod",
			Params: openapi.DeleteConnectorInstanceParams{
				ScopeKind: openapi.ConnectorScopeKindCompany, Company: ptr("acme"),
			},
		})
	if err != nil {
		t.Fatalf("DeleteConnectorInstance: %v", err)
	}
	if _, ok := resp.(openapi.DeleteConnectorInstance204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	if spy.deleted != "prod" || spy.putKind != settings.ScopeCompany ||
		spy.putScope != (domain.Scope{Company: "acme"}) {
		t.Fatalf("deleted=%s kind=%s scope=%s", spy.deleted, spy.putKind, spy.putScope)
	}
}
