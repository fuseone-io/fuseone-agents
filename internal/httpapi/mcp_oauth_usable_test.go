package httpapi

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// A grant with nothing a request could carry is the caller's mistake to
// correct, said as such: invalid input, not a refusal by an upstream nobody
// called.
func TestPutMCPServer_anOAuthGrantWithNothingToSend_isInvalidInput(t *testing.T) {
	s := mcpWorld(t)
	body := openapi.PutMCPServerJSONRequestBody{
		Transport: ptr(openapi.TransportHttp), Url: ptr("https://mcp.example.com/remote"),
		Oauth: &openapi.MCPOAuthGrant{
			ClientID: ptr("agents"), TokenURL: ptr("https://idp.example.com/token"),
		},
	}
	resp, err := s.PutMCPServer(as(domain.RoleCurator), openapi.PutMCPServerRequestObject{Name: "remote", Body: &body})
	bad, ok := resp.(openapi.PutMCPServer400ApplicationProblemPlusJSONResponse)
	if err != nil || !ok || valueOr(bad.Type) != "fuseone:invalid-input" {
		t.Fatalf("PutMCPServer = %#v, %v; want 400 invalid-input", resp, err)
	}
}
