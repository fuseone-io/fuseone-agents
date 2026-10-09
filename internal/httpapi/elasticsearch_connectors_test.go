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

// The configuration crosses whole, and the password travels beside it to be
// sealed — answered afterwards only as hasToken, never as bytes.
func TestPutConnectorInstance_carriesTheElasticsearchBoundary(t *testing.T) {
	t.Parallel()
	spy := &connectorInstanceSpy{}
	resp, err := NewServer(ledger.NewMemory(), "test").WithConnectorInstances(spy).
		PutConnectorInstance(as(domain.RoleCurator), openapi.PutConnectorInstanceRequestObject{
			Name: "gw-logs",
			Body: &openapi.PutConnectorInstanceJSONRequestBody{
				Connector: "elasticsearch", ScopeKind: openapi.ConnectorScopeKindArea,
				Company: ptr("acme"), Area: ptr("platform"),
				Token: ptr("CANARY-pass"),
				Elasticsearch: &openapi.ConnectorElasticsearchConfig{
					BaseUrl: "http://search.internal:9200", Username: "reader",
					Index: "logs-*", PathField: ptr("url.path"), MaxWindowDays: ptr(14),
				},
			},
		})
	if err != nil {
		t.Fatalf("PutConnectorInstance: %v", err)
	}
	if _, ok := resp.(openapi.PutConnectorInstance204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	cfg := spy.put.Elasticsearch
	if cfg.Index != "logs-*" || cfg.PathField != "url.path" || cfg.MaxWindowDays != 14 {
		t.Fatalf("Elasticsearch = %+v", cfg)
	}
	if spy.putToken == nil || *spy.putToken != "CANARY-pass" {
		t.Fatal("the password did not travel to be sealed")
	}
}

func TestConnectorInstanceResponses_elasticsearchNeverReturnsThePassword(t *testing.T) {
	t.Parallel()
	configured := connectortools.ConfiguredInstance{
		Instance: connectortools.Instance{
			Name: "gw-logs", Connector: "elasticsearch", Enabled: true,
			Scope: domain.Scope{Company: "acme", Area: "platform"},
			Elasticsearch: connectortools.ElasticsearchConfig{
				BaseURL: "http://search.internal:9200", Username: "reader", Index: "logs-*",
			},
			Token: "CANARY-pass", HasToken: true,
		},
		ScopeKind: settings.ScopeArea, HasToken: true,
	}
	for name, body := range map[string]any{
		"listing": connectorInstanceResponse(configured),
		"detail":  connectorInstanceDetailResponse(configured),
	} {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		if strings.Contains(string(raw), "CANARY") {
			t.Fatalf("the %s exposed the password: %s", name, raw)
		}
		if !strings.Contains(string(raw), "logs-*") {
			t.Fatalf("the %s lost the index: %s", name, raw)
		}
	}
}
