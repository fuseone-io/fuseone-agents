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

func evidenceAPIConfig(t *testing.T) *openapi.ConnectorEvidenceConfig {
	t.Helper()
	var out openapi.ConnectorEvidenceConfig
	if err := json.Unmarshal([]byte(`{
		"baseUrl": "https://source.example.com", "authHeader": "Authorization", "authPrefix": "ApiToken ",
		"control": "all", "rows": "result", "maxEvents": 5000,
		"refusal": {"field": "ok", "value": "0"},
		"gap": {"field": "bypass_traffic", "value": "yes"},
		"paging": {"kind": "offset", "limitParam": "limit", "pageSize": 100, "offsetParam": "offset"},
		"window": {"kind": "period", "periodParam": "timeperiod", "periods": [3600, 86400]},
		"queries": [
			{"id": "all", "description": "Everything.", "path": "/events", "fields": ["_id"], "idField": "_id"},
			{"id": "by_user", "description": "One user.", "path": "/events",
			 "filters": [{"name": "query", "value": "user eq '{user}'"}],
			 "parameters": [{"name": "user", "pattern": "[a-z]+"}],
			 "fields": ["_id", "timestamp"], "idField": "_id", "timeField": "timestamp", "windowless": true}
		]}`), &out); err != nil {
		t.Fatal(err)
	}
	return &out
}

// The whole declaration crosses, field by field, and the token beside it: config
// fields die in the middle layer, so this asserts what arrives and that it validates.
func TestPutConnectorInstance_carriesTheEvidenceDeclaration(t *testing.T) {
	t.Parallel()
	spy := &connectorInstanceSpy{}
	resp, err := NewServer(ledger.NewMemory(), "test").WithConnectorInstances(spy).
		PutConnectorInstance(as(domain.RoleCurator), openapi.PutConnectorInstanceRequestObject{
			Name: "gateway",
			Body: &openapi.PutConnectorInstanceJSONRequestBody{
				Connector: "evidence", ScopeKind: openapi.ConnectorScopeKindArea,
				Company: ptr("acme"), Area: ptr("cyber"), Token: ptr("CANARY-token"),
				Evidence: evidenceAPIConfig(t),
			},
		})
	if err != nil {
		t.Fatalf("PutConnectorInstance: %v", err)
	}
	if _, ok := resp.(openapi.PutConnectorInstance204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	got := spy.put.Evidence
	if got.AuthPrefix != "ApiToken " || got.Gap.Value != "yes" || got.Refusal.Field != "ok" ||
		got.Paging.OffsetParam != "offset" || got.Window.Periods[1] != 86400 || got.MaxEvents != 5000 {
		t.Fatalf("Evidence = %+v", got)
	}
	query := got.Queries[1]
	if query.Parameters[0].Pattern != "[a-z]+" || query.Filters[0].Value != "user eq '{user}'" ||
		query.TimeField != "timestamp" || !query.Windowless {
		t.Fatalf("query = %+v", query)
	}
	if err := connectortools.ValidateInstanceConfig(spy.put); err != nil {
		t.Fatalf("what crossed does not validate: %v", err)
	}
	if spy.putToken == nil || *spy.putToken != "CANARY-token" {
		t.Fatal("the token did not travel to be sealed")
	}
}

// Read, edit, write must not lose a field, and the token stays out of both.
func TestConnectorInstanceResponses_evidenceRoundTripsWithoutTheToken(t *testing.T) {
	t.Parallel()
	spy := &connectorInstanceSpy{}
	_, err := NewServer(ledger.NewMemory(), "test").WithConnectorInstances(spy).
		PutConnectorInstance(as(domain.RoleCurator), openapi.PutConnectorInstanceRequestObject{
			Name: "gateway",
			Body: &openapi.PutConnectorInstanceJSONRequestBody{
				Connector: "evidence", ScopeKind: openapi.ConnectorScopeKindArea,
				Company: ptr("acme"), Area: ptr("cyber"), Evidence: evidenceAPIConfig(t),
			},
		})
	if err != nil {
		t.Fatal(err)
	}
	instance := spy.put
	instance.Token, instance.HasToken = "CANARY-token", true
	configured := connectortools.ConfiguredInstance{
		Instance: instance, ScopeKind: settings.ScopeArea, HasToken: true,
	}
	want, _ := json.Marshal(evidenceAPIConfig(t))
	for name, body := range map[string]any{
		"listing": connectorInstanceResponse(configured),
		"detail":  connectorInstanceDetailResponse(configured),
	} {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		if strings.Contains(string(raw), "CANARY") {
			t.Fatalf("the %s exposed the token: %s", name, raw)
		}
		var decoded struct {
			Evidence json.RawMessage `json:"evidence"`
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		if a, b := canonicalJSON(t, want), canonicalJSON(t, decoded.Evidence); a != b {
			t.Fatalf("the %s changed the declaration:\n want %s\n got  %s", name, a, b)
		}
	}
}

func canonicalJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	out, _ := json.Marshal(value)
	return string(out)
}
