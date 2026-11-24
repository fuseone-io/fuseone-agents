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
The browser sign-in configuration through the door: written with the server,
read back with everything but the client secret, which a reader only learns
is held, kept by every write that does not mention it, and refused as invalid
input where no sign-in could use it.
*/

// configureSignIn writes the sign-in through the store, for the tests about
// reading it: what the door writes is tested through the door, below.
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

// What a read answers, sent back as it is, changes nothing: the secret it does
// not carry is kept, not taken for a removal.
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

// putSignIn writes the sign-in through the door.
func putSignIn(t *testing.T, s *Server, login *openapi.MCPOAuthLoginInput) openapi.PutMCPServerResponseObject {
	t.Helper()
	body := openapi.PutMCPServerJSONRequestBody{
		Transport: ptr(openapi.TransportHttp), Url: ptr("https://mcp.example.com/signin"),
	}
	if login != nil {
		raw, err := json.Marshal(login)
		if err != nil {
			t.Fatalf("encode the sign-in: %v", err)
		}
		body.OauthLogin = raw
	}
	resp, err := s.PutMCPServer(as(domain.RoleCurator), openapi.PutMCPServerRequestObject{Name: "signin", Body: &body})
	if err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	return resp
}

func readSignIn(t *testing.T, s *Server) *openapi.MCPOAuthLogin {
	t.Helper()
	resp, err := s.GetMCPServer(as(domain.RoleCurator), openapi.GetMCPServerRequestObject{Name: "signin"})
	got, ok := resp.(openapi.GetMCPServer200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("GetMCPServer = %#v, %v", resp, err)
	}
	return got.Body.OauthLogin
}

func TestPutMCPServer_aSignIn_isWrittenThroughTheDoor_withItsSecretHeld(t *testing.T) {
	s := mcpWorld(t)
	if resp := putSignIn(t, s, &openapi.MCPOAuthLoginInput{ClientID: ptr("agents"), ClientSecret: ptr("s3cret")}); func() bool {
		_, ok := resp.(openapi.PutMCPServer204Response)
		return !ok
	}() {
		t.Fatalf("PutMCPServer = %#v, want 204", resp)
	}
	if login := readSignIn(t, s); login == nil || login.ClientID != "agents" || !login.HasClientSecret {
		t.Errorf("oauthLogin = %+v, want agents with a secret held", login)
	}
}

func TestPutMCPServer_aSignInNoSignInCouldUse_isInvalidInput(t *testing.T) {
	s := mcpWorld(t)
	resp := putSignIn(t, s, &openapi.MCPOAuthLoginInput{
		ClientID: ptr("agents"), TokenAuthMethod: ptr(openapi.MCPOAuthLoginInputTokenAuthMethodClientSecretPost),
	})
	bad, ok := resp.(openapi.PutMCPServer400ApplicationProblemPlusJSONResponse)
	if !ok || valueOr(bad.Type) != "fuseone:invalid-input" {
		t.Errorf("PutMCPServer = %#v, want 400 invalid-input", resp)
	}
}

// An empty object is the way to take a configuration back.
func TestPutMCPServer_anEmptySignIn_removesTheStoredOne(t *testing.T) {
	s := mcpWorld(t)
	putSignIn(t, s, &openapi.MCPOAuthLoginInput{ClientID: ptr("agents")})
	if readSignIn(t, s) == nil {
		t.Fatal("the sign-in was not stored")
	}
	putSignIn(t, s, &openapi.MCPOAuthLoginInput{})
	if login := readSignIn(t, s); login != nil {
		t.Errorf("oauthLogin = %+v after an empty one, want none", login)
	}
}

// Only an object with nothing in it takes the sign-in back. One that decodes
// to nothing for another reason — a field sent as null, or only fields the
// door does not know — is a mistake, refused, and the sign-in kept.
func TestPutMCPServer_aSignInThatOnlyLooksEmpty_isRefusedAndKeepsTheStoredOne(t *testing.T) {
	s := mcpWorld(t)
	putSignIn(t, s, &openapi.MCPOAuthLoginInput{ClientID: ptr("agents"), ClientSecret: ptr("s3cret")})
	for name, raw := range map[string]string{
		"a client ID sent as null": `{"clientID":null}`,
		"only unknown fields":      `{"clientId":"agents"}`,
		"the object itself null":   `null`,
		"a secret sent as null":    `{"clientID":"agents","clientSecret":null}`,
		// Ignored, it would save a sign-in without the endpoint meant.
		"a misspelt field beside a known one": `{"clientID":"agents","tokenUrl":"https://mcp.example.com/token"}`,
	} {
		var body openapi.PutMCPServerJSONRequestBody
		if err := json.Unmarshal([]byte(`{"transport":"http","url":"https://mcp.example.com/signin","oauthLogin":`+raw+`}`), &body); err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		resp, err := s.PutMCPServer(as(domain.RoleCurator), openapi.PutMCPServerRequestObject{Name: "signin", Body: &body})
		if bad, ok := resp.(openapi.PutMCPServer400ApplicationProblemPlusJSONResponse); err != nil || !ok || valueOr(bad.Type) != "fuseone:invalid-input" {
			t.Errorf("%s: PutMCPServer = %#v, %v; want 400 invalid-input", name, resp, err)
		}
		if login := readSignIn(t, s); login == nil || !login.HasClientSecret {
			t.Errorf("%s: oauthLogin = %+v; want the stored one kept, with its secret", name, login)
		}
	}
}
