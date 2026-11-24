package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
The browser sign-in configuration through the door: read back with everything
but the client secret, which a reader only learns is held, and kept by every
write that does not mention it. This release reads it and writes none: every
replica keeps it before any can write it.
*/

// configureSignIn writes the sign-in where only the store can, this release:
// the door reads it and keeps it, and takes no write of it yet.
func configureSignIn(t *testing.T, s *Server, login domain.MCPOAuthLoginPatch) {
	t.Helper()
	integrations, ok := s.integrations.(*admin.Integrations)
	if !ok {
		t.Fatalf("integrations = %T, want the store", s.integrations)
	}
	if _, err := integrations.WriteMCPServer(context.Background(), admin.MCPServerWrite{
		By: "usr_ana", Scope: adminScope,
		Server: domain.MCPServer{Name: "signin", Transport: domain.TransportHTTP,
			URL: "https://mcp.example.com/signin", Enabled: true},
		Login: &login,
	}); err != nil {
		t.Fatalf("WriteMCPServer: %v", err)
	}
}

func TestPutMCPServer_aSignInConfiguration_isReadBackWithoutItsSecret(t *testing.T) {
	s := mcpWorld(t)
	configureSignIn(t, s, domain.MCPOAuthLoginPatch{
		ClientID: "agents", ClientSecret: ptr("s3cret"), Scopes: []string{"openid"},
	})

	resp, err := s.GetMCPServer(as(domain.RoleCurator), openapi.GetMCPServerRequestObject{Name: "signin"})
	if err != nil {
		t.Fatalf("GetMCPServer: %v", err)
	}
	got, ok := resp.(openapi.GetMCPServer200JSONResponse)
	if !ok || got.Body.OauthLogin == nil {
		t.Fatalf("GetMCPServer = %#v, want the server with its sign-in", resp)
	}
	login := got.Body.OauthLogin
	if login.ClientID != "agents" || !login.HasClientSecret ||
		login.TokenAuthMethod != openapi.MCPOAuthLoginTokenAuthMethodClientSecretBasic {
		t.Errorf("oauthLogin = %+v, want agents, a secret held, client_secret_basic", login)
	}
	raw, _ := json.Marshal(got.Body)
	if strings.Contains(string(raw), "s3cret") {
		t.Error("the client secret came back through the read")
	}
}

// What a read answers, sent back as it is, changes nothing. This release's
// write does not know the field: it keeps the sign-in, as the release after
// it, which writes it, must also find it kept.
func TestPutMCPServer_theReadSentBack_keepsTheSignInAndItsSecret(t *testing.T) {
	s := mcpWorld(t)
	configureSignIn(t, s, domain.MCPOAuthLoginPatch{ClientID: "agents", ClientSecret: ptr("s3cret")})
	read := func() openapi.GetMCPServer200JSONResponse {
		t.Helper()
		resp, err := s.GetMCPServer(as(domain.RoleCurator), openapi.GetMCPServerRequestObject{Name: "signin"})
		got, ok := resp.(openapi.GetMCPServer200JSONResponse)
		if err != nil || !ok {
			t.Fatalf("GetMCPServer = %#v, %v", resp, err)
		}
		return got
	}
	before := read()
	raw, _ := json.Marshal(before.Body)
	var body openapi.PutMCPServerJSONRequestBody
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("the read as a write: %v", err)
	}
	resp, err := s.PutMCPServer(as(domain.RoleCurator), openapi.PutMCPServerRequestObject{Name: "signin", Body: &body})
	if _, ok := resp.(openapi.PutMCPServer204Response); err != nil || !ok {
		t.Fatalf("PutMCPServer = %#v, %v; want 204", resp, err)
	}
	after := read()
	if after.Headers.ETag != before.Headers.ETag || after.Body.OauthLogin == nil || !after.Body.OauthLogin.HasClientSecret {
		t.Errorf("after the read sent back: ETag %s (was %s), sign-in %+v; want both unchanged",
			after.Headers.ETag, before.Headers.ETag, after.Body.OauthLogin)
	}
}

// Credentials says which servers people sign in to, and that every call on
// them takes the person's own credential, with no recipe to say so.
func TestListMCPUserCredentials_aServerPeopleSignInTo_isOfferedAsTakingTheirOwn(t *testing.T) {
	s := mcpWorld(t)
	configureSignIn(t, s, domain.MCPOAuthLoginPatch{ClientID: "agents"})
	putMCP(t, s, as(domain.RoleCurator), "plain", atInstallation, false)

	resp, err := s.ListMCPUserCredentials(as(domain.RoleCurator), openapi.ListMCPUserCredentialsRequestObject{})
	body, ok := resp.(openapi.ListMCPUserCredentials200JSONResponse)
	if err != nil || !ok || body.Servers == nil {
		t.Fatalf("ListMCPUserCredentials = %#v, %v", resp, err)
	}
	got := map[string][2]bool{}
	for _, server := range *body.Servers {
		got[server.Name] = [2]bool{server.SignIn, server.RequiresPersonal}
	}
	if got["signin"] != [2]bool{true, true} || got["plain"] != [2]bool{false, false} {
		t.Errorf("servers = %v, want signin signed in and personal, plain neither", got)
	}
}
