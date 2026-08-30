package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
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

func sqlBody() *openapi.ConnectorSQLInput {
	return &openapi.ConnectorSQLInput{
		Driver: openapi.ConnectorSQLInputDriverPostgres,
		Host:   "db.internal", Port: 5432, Database: "appx",
		CredentialSource: openapi.ConnectorCredentialSource{
			Kind: openapi.VaultDatabaseRole, VaultInstance: "prod",
			Mount: "database", Role: "app-x-readonly",
		},
		Templates: []openapi.ConnectorSQLTemplate{{
			Id: "orders_by_customer", Sql: "select id from orders where customer_id = $1",
			Parameters: []openapi.ConnectorSQLParameter{
				{Name: "customer_id", Type: openapi.Text},
			},
			TimeoutSeconds: 10, MaxRows: 200, MaxBytes: 65536,
		}},
	}
}

/*
What the API accepts is what the validator requires.

The driver and the templates became required in the same commit that added
them to the domain, and the contract did not carry either — so every SQL
configuration sent to this endpoint was refused for a field the caller had no
way to send. Nothing caught it because no test crossed the boundary.
*/
func TestPutConnectorInstance_carriesTheDriverAndTheRegisteredTemplates(t *testing.T) {
	t.Parallel()

	spy := &connectorInstanceSpy{}
	resp, err := NewServer(ledger.NewMemory(), "test").WithConnectorInstances(spy).
		PutConnectorInstance(as(domain.RoleCurator), openapi.PutConnectorInstanceRequestObject{
			Name: "app-x",
			Body: &openapi.PutConnectorInstanceJSONRequestBody{
				Connector: "sql", ScopeKind: openapi.ConnectorScopeKindArea,
				Company: ptr("acme"), Area: ptr("platform"), Sql: sqlBody(),
			},
		})
	if err != nil {
		t.Fatalf("PutConnectorInstance: %v", err)
	}
	if _, ok := resp.(openapi.PutConnectorInstance204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	stored := spy.put.SQL
	if stored.Driver != connectortools.SQLDriverPostgres {
		t.Fatalf("driver = %q, want postgres", stored.Driver)
	}
	if len(stored.Templates) != 1 || stored.Templates[0].ID != "orders_by_customer" ||
		stored.Templates[0].MaxRows != 200 {
		t.Fatalf("templates = %+v, want the registered query", stored.Templates)
	}
	if len(stored.Templates[0].Parameters) != 1 ||
		stored.Templates[0].Parameters[0].Type != connectortools.SQLParamText {
		t.Fatalf("parameters = %+v, want the declared type", stored.Templates[0].Parameters)
	}
}

/*
Listing reports what a template is, never what it says.

Listing connector instances needs only tool:read, and a registered query names
the tables, columns and filters of a customer database. The digest lets an
operator confirm the configuration matches what they wrote without publishing
it to everyone who can read the tool catalogue.
*/
func TestListConnectorInstances_reportsATemplateDigestAndNotItsQuery(t *testing.T) {
	t.Parallel()

	const query = "select id from orders where customer_id = $1"
	spy := &connectorInstanceSpy{items: []connectortools.ConfiguredInstance{{
		Instance: connectortools.Instance{
			Name: "app-x", Connector: "sql", Enabled: true,
			Scope: domain.Scope{Company: "acme", Area: "platform"},
			SQL: connectortools.SQLConfig{
				Driver: connectortools.SQLDriverPostgres,
				Host:   "db.internal", Port: 5432, Database: "appx",
				Templates: []connectortools.SQLTemplate{{
					ID: "orders_by_customer", SQL: query,
					Parameters:     []connectortools.SQLParameter{{Name: "customer_id", Type: connectortools.SQLParamText}},
					TimeoutSeconds: 10, MaxRows: 200, MaxBytes: 65536,
				}},
			},
		},
		ScopeKind: settings.ScopeArea,
	}}}
	resp, err := NewServer(ledger.NewMemory(), "test").WithConnectorInstances(spy).
		ListConnectorInstances(as(domain.RoleCurator), openapi.ListConnectorInstancesRequestObject{})
	if err != nil {
		t.Fatalf("ListConnectorInstances: %v", err)
	}
	body := resp.(openapi.ListConnectorInstances200JSONResponse)
	rendered, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(rendered), "customer_id = $1") {
		t.Fatalf("the listing published the query: %s", rendered)
	}
	sql := body.Items[0].Sql
	if sql == nil || len(sql.Templates) != 1 {
		t.Fatalf("sql = %+v, want the template summary", sql)
	}
	want := fmt.Sprintf("%x", sha256.Sum256([]byte(query)))
	if sql.Templates[0].SqlDigest != want {
		t.Fatalf("digest = %q, want %q", sql.Templates[0].SqlDigest, want)
	}
}
