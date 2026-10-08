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

// The configuration crosses the boundary whole — protected ranges included,
// because a lost range is a hole in the one guard the operator authored —
// and the token travels beside it to be sealed, never inside it.
func TestPutConnectorInstance_carriesTheCloudflareBoundaryAndTheTokenToSeal(t *testing.T) {
	t.Parallel()

	spy := &connectorInstanceSpy{}
	resp, err := NewServer(ledger.NewMemory(), "test").WithConnectorInstances(spy).
		PutConnectorInstance(as(domain.RoleCurator), openapi.PutConnectorInstanceRequestObject{
			Name: "edge",
			Body: &openapi.PutConnectorInstanceJSONRequestBody{
				Connector: "cloudflare", ScopeKind: openapi.ConnectorScopeKindArea,
				Company: ptr("acme"), Area: ptr("platform"),
				Token: ptr("CANARY-cf-token"),
				Cloudflare: &openapi.ConnectorCloudflareConfig{
					AccountId:       "acct1234",
					ListId:          "list5678",
					ProtectedRanges: ptr([]string{"203.0.113.0/24", "2001:db8::/32"}),
					MaxBlocksPerDay: ptr(50),
				},
			},
		})
	if err != nil {
		t.Fatalf("PutConnectorInstance: %v", err)
	}
	if _, ok := resp.(openapi.PutConnectorInstance204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	cfg := spy.put.Cloudflare
	if cfg.AccountID != "acct1234" || cfg.ListID != "list5678" ||
		len(cfg.ProtectedRanges) != 2 || cfg.MaxBlocksPerDay != 50 {
		t.Fatalf("Cloudflare = %+v, want the authored boundary", cfg)
	}
	if spy.putToken == nil || *spy.putToken != "CANARY-cf-token" {
		t.Fatal("the token did not travel to be sealed")
	}
}

// The listing and the detail both say hasToken and never the token; the
// non-secret configuration is present in both, since nothing in it is a
// credential.
func TestConnectorInstanceResponses_cloudflareNeverReturnsTheToken(t *testing.T) {
	t.Parallel()

	configured := connectortools.ConfiguredInstance{
		Instance: connectortools.Instance{
			Name: "edge", Connector: "cloudflare", Enabled: true,
			Scope: domain.Scope{Company: "acme", Area: "platform"},
			Cloudflare: connectortools.CloudflareConfig{
				AccountID: "acct1234", ListID: "list5678",
				ProtectedRanges: []string{"203.0.113.0/24"},
			},
			Token: "CANARY-cf-token", HasToken: true,
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
			t.Fatalf("the %s exposed the token: %s", name, raw)
		}
		if !strings.Contains(string(raw), "203.0.113.0/24") {
			t.Fatalf("the %s lost the protected ranges: %s", name, raw)
		}
	}
}
