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
			{"id": "all", "description": "Everything.", "path": "/events", "fields": ["_id", "bypass_traffic"], "idField": "_id"},
			{"id": "by_user", "description": "One user.", "path": "/events",
			 "filters": [{"name": "query", "value": "user eq '{user}'"}],
			 "parameters": [{"name": "user", "pattern": "[a-z]+"}],
			 "fields": ["_id", "timestamp", "bypass_traffic"], "idField": "_id", "timeField": "timestamp", "windowless": true}
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

func configuredEvidence(t *testing.T) connectortools.ConfiguredInstance {
	t.Helper()
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
	return connectortools.ConfiguredInstance{
		Instance: instance, ScopeKind: settings.ScopeArea, HasToken: true,
	}
}

// Read, edit, write must not lose a field, and the token stays out.
func TestConnectorInstanceDetail_evidenceRoundTripsWithoutTheToken(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(connectorInstanceDetailResponse(configuredEvidence(t)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "CANARY") {
		t.Fatalf("the detail exposed the token: %s", raw)
	}
	var decoded struct {
		Evidence json.RawMessage `json:"evidence"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(evidenceAPIConfig(t))
	if a, b := canonicalJSON(t, want), canonicalJSON(t, decoded.Evidence); a != b {
		t.Fatalf("the detail changed the declaration:\n want %s\n got  %s", a, b)
	}
}

// The listing answers anyone who may read tools, Authors and Auditors included.
// Which paths, filters, patterns and fields a source reads is for whoever configures
// it, as with SQL's query text, so the listing says what it is and how much.
func TestConnectorInstanceListing_evidenceKeepsTheDeclarationToConfigurers(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(connectorInstanceResponse(configuredEvidence(t)))
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{
		"CANARY", "/events", "user eq", "[a-z]+", "bypass_traffic", "timestamp", "Authorization", "by_user",
	} {
		if strings.Contains(string(raw), private) {
			t.Errorf("the listing exposed %q: %s", private, raw)
		}
	}
	var decoded struct {
		Evidence struct {
			BaseURL string `json:"baseUrl"`
			Queries int    `json:"queries"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	if decoded.Evidence.BaseURL != "https://source.example.com" || decoded.Evidence.Queries != 2 {
		t.Fatalf("summary = %+v", decoded.Evidence)
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
