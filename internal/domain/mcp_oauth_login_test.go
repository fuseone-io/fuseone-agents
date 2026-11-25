package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
A server's browser sign-in configuration is written as a patch: omitted keeps
what is stored, an empty one removes it, and the client secret has a gesture
of its own: omitted keeps it, empty removes it. A token endpoint method that
sends a secret needs one to be held.
*/

func strp(s string) *string { return &s }

func TestMCPOAuthLoginPatch_apply(t *testing.T) {
	t.Parallel()
	stored := &domain.MCPOAuthLogin{
		ClientID: "agents", TokenAuthMethod: domain.TokenAuthClientSecretBasic, HasClientSecret: true,
	}
	for _, tc := range []struct {
		name       string
		patch      domain.MCPOAuthLoginPatch
		want       *domain.MCPOAuthLogin
		wantSecret domain.LoginSecretChange
	}{
		{
			name:       "an empty patch removes the configuration and its secret",
			patch:      domain.MCPOAuthLoginPatch{Remove: true},
			want:       nil,
			wantSecret: domain.LoginSecretChange{Remove: true},
		},
		{
			name:  "a secret left out keeps the one held",
			patch: domain.MCPOAuthLoginPatch{ClientID: "agents-2"},
			want: &domain.MCPOAuthLogin{
				ClientID: "agents-2", TokenAuthMethod: domain.TokenAuthClientSecretBasic, HasClientSecret: true,
			},
		},
		{
			name:  "an empty secret removes it, and the method falls back to none",
			patch: domain.MCPOAuthLoginPatch{ClientID: "agents", ClientSecret: strp("")},
			want: &domain.MCPOAuthLogin{
				ClientID: "agents", TokenAuthMethod: domain.TokenAuthNone,
			},
			wantSecret: domain.LoginSecretChange{Remove: true},
		},
		{
			name:  "a new secret is held, sent by basic unless another method is named",
			patch: domain.MCPOAuthLoginPatch{ClientID: "agents", ClientSecret: strp("s3cret")},
			want: &domain.MCPOAuthLogin{
				ClientID: "agents", TokenAuthMethod: domain.TokenAuthClientSecretBasic, HasClientSecret: true,
			},
			wantSecret: domain.LoginSecretChange{Set: strp("s3cret")},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, secret, err := tc.patch.Apply(stored)
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if !got.Equal(tc.want) {
				t.Errorf("login = %+v, want %+v", got, tc.want)
			}
			if !secret.Equal(tc.wantSecret) {
				t.Errorf("secret change = %+v, want %+v", secret, tc.wantSecret)
			}
		})
	}
}

func TestMCPOAuthLoginPatch_firstConfigurationWithNoSecret_usesNoClientAuthentication(t *testing.T) {
	t.Parallel()
	got, _, err := domain.MCPOAuthLoginPatch{ClientID: "public"}.Apply(nil)
	if err != nil || got.TokenAuthMethod != domain.TokenAuthNone || got.HasClientSecret {
		t.Errorf("Apply = %+v, %v; want method none and no secret", got, err)
	}
}

func TestMCPOAuthLoginPatch_refusesWhatCannotSignIn(t *testing.T) {
	t.Parallel()
	for name, patch := range map[string]domain.MCPOAuthLoginPatch{
		"no client id":                           {ClientID: "  "},
		"an unknown token endpoint method":       {ClientID: "agents", TokenAuthMethod: "private_key_jwt"},
		"a secret method with no secret":         {ClientID: "agents", TokenAuthMethod: domain.TokenAuthClientSecretPost},
		"a resource that is not an absolute URI": {ClientID: "agents", Resource: "mcp server"},
		"a resource with a fragment":             {ClientID: "agents", Resource: "https://mcp.example.com/#x"},
		"a token endpoint with a query":          {ClientID: "agents", TokenURL: "https://issuer.example.com/token?key=x"},
		"an endpoint with a fragment":            {ClientID: "agents", AuthorizeURL: "https://issuer.example.com/authorize#x"},
	} {
		if _, _, err := patch.Apply(nil); !errors.Is(err, domain.ErrMCPOAuthLoginInvalid) {
			t.Errorf("%s: err = %v, want ErrMCPOAuthLoginInvalid", name, err)
		}
	}
}

// Every tool call on a server people sign in to acts as the caller: the
// recipe saying so, or a sign-in configured, and only for a remote server.
func TestMCPServer_takesPersonalCredential(t *testing.T) {
	t.Parallel()
	remote := domain.MCPServer{Name: "s", Transport: domain.TransportHTTP}
	signedIn := remote
	signedIn.OAuthLogin = &domain.MCPOAuthLogin{ClientID: "agents", TokenAuthMethod: domain.TokenAuthNone}
	local := domain.MCPServer{Name: "s", Transport: domain.TransportStdio, OAuthLogin: signedIn.OAuthLogin}
	for _, tc := range []struct {
		name     string
		server   domain.MCPServer
		userOnly bool
		want     bool
	}{
		{"a recipe for people alone", remote, true, true},
		{"a sign-in configured", signedIn, false, true},
		{"neither", remote, false, false},
		{"a local process", local, true, false},
	} {
		if got := tc.server.TakesPersonalCredential(tc.userOnly); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Stored in the open and answered on every read, an endpoint carrying a
// credential would publish it. Refused, and the refusal never repeats it.
func TestMCPOAuthLoginPatch_anEndpointCarryingACredential_isRefusedWithoutRepeatingIt(t *testing.T) {
	t.Parallel()
	const canary = "canary-7f3a"
	for name, patch := range map[string]domain.MCPOAuthLoginPatch{
		"the token endpoint":     {ClientID: "agents", TokenURL: "https://user:" + canary + "@issuer.example.com/token"},
		"the authorize endpoint": {ClientID: "agents", AuthorizeURL: "https://user:" + canary + "@issuer.example.com/authorize"},
		"the resource":           {ClientID: "agents", Resource: "https://user:" + canary + "@mcp.example.com/"},
	} {
		_, _, err := patch.Apply(nil)
		if !errors.Is(err, domain.ErrMCPOAuthLoginInvalid) {
			t.Errorf("%s: err = %v, want ErrMCPOAuthLoginInvalid", name, err)
		}
		if err != nil && strings.Contains(err.Error(), canary) {
			t.Errorf("%s: the refusal repeats the credential: %v", name, err)
		}
	}
}

// A sign-in's authorization endpoint may keep a query of its own (a policy
// name, say): it is the address every person's browser opens, public by what
// it is.
func TestMCPOAuthLoginPatch_anAuthorizeEndpointsQuery_isKept(t *testing.T) {
	t.Parallel()
	got, _, err := domain.MCPOAuthLoginPatch{ClientID: "agents",
		AuthorizeURL: "https://issuer.example.com/authorize?p=signin"}.Apply(nil)
	if err != nil || got.AuthorizeURL != "https://issuer.example.com/authorize?p=signin" {
		t.Errorf("Apply = %+v, %v; want the query kept", got, err)
	}
}

// RFC 8707 takes any absolute URI as a resource, a URN among them: it names
// the server and is never fetched, so it needs no host.
func TestMCPOAuthLoginPatch_aResourceURNWithNoHost_isKept(t *testing.T) {
	t.Parallel()
	got, _, err := domain.MCPOAuthLoginPatch{ClientID: "agents", Resource: "urn:example:service"}.Apply(nil)
	if err != nil || got.Resource != "urn:example:service" {
		t.Errorf("Apply = %+v, %v; want the URN kept", got, err)
	}
}

// A grant is bound to the configuration it was got under, all but the secret,
// and to where its token requests actually go: the token endpoint and the
// resource in effect, configured or found. Rotating the secret alone keeps
// the identity; changing anything a token request is sent to or by moves it.
func TestMCPOAuthLogin_identity_movesWithAllButTheSecret(t *testing.T) {
	t.Parallel()
	const token, resource = "https://idp.example.com/token", "https://mcp.example.com/mcp"
	base := domain.MCPOAuthLogin{ClientID: "agents", Scopes: []string{"openid"},
		TokenAuthMethod: domain.TokenAuthClientSecretBasic, HasClientSecret: true}
	// Whether a secret is held, and which, are not part of it.
	same := base
	same.HasClientSecret = false
	if base.Identity(token, resource) != same.Identity(token, resource) || base.Identity(token, resource) == "" {
		t.Fatalf("identity %q vs %q; want equal and set", base.Identity(token, resource), same.Identity(token, resource))
	}
	for name, change := range map[string]func(*domain.MCPOAuthLogin){
		"the client":              func(l *domain.MCPOAuthLogin) { l.ClientID = "agents-2" },
		"the configured token":    func(l *domain.MCPOAuthLogin) { l.TokenURL = token },
		"the authorize one":       func(l *domain.MCPOAuthLogin) { l.AuthorizeURL = "https://idp.example.com/auth" },
		"the configured resource": func(l *domain.MCPOAuthLogin) { l.Resource = resource },
		"the method":              func(l *domain.MCPOAuthLogin) { l.TokenAuthMethod = domain.TokenAuthClientSecretPost },
		"the scopes":              func(l *domain.MCPOAuthLogin) { l.Scopes = []string{"openid", "profile"} },
	} {
		changed := base
		change(&changed)
		if changed.Identity(token, resource) == base.Identity(token, resource) {
			t.Errorf("%s changed and the identity did not", name)
		}
	}
	// Found rather than configured: what is in effect is in it too.
	if base.Identity("https://idp.example.com/token-2", resource) == base.Identity(token, resource) {
		t.Error("the token endpoint in effect changed and the identity did not")
	}
	if base.Identity(token, "https://mcp.example.com/other") == base.Identity(token, resource) {
		t.Error("the resource in effect changed and the identity did not")
	}
}
