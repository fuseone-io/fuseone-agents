package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/mcpauth"
)

// signInLifetime is how long a browser sign-in waits for the browser.
const signInLifetime = 10 * time.Minute

// callbackPath is where the browser comes back to, under the base URL.
const callbackPath = "/auth/mcp/callback"

// MCPSignIns is what a browser sign-in needs of the store, declared here by
// its consumer.
type MCPSignIns interface {
	MCPSignInServer(ctx context.Context, name string, visible []domain.Scope) (domain.MCPServer, error)
	MCPOAuthLoginClient(ctx context.Context, runtime domain.Scope, name string) (domain.MCPOAuthLogin, string, error)
	BeginMCPOAuthFlow(ctx context.Context, f admin.MCPOAuthFlow, now time.Time) error
	ConsumeMCPOAuthFlow(ctx context.Context, state string, principal domain.UserID, now time.Time) (admin.MCPOAuthFlow, error)
}

// MCPSignInConfig wires the browser sign-in. Client is the outbound client for
// one server: the installation's network guard and the server's egress
// policy, both the caller's to hold. Now and Random are the edge's clock and
// randomness, so what is recorded can be reproduced in a test.
type MCPSignInConfig struct {
	Store   MCPSignIns
	Client  func(domain.MCPServer) *http.Client
	BaseURL string
	Now     func() time.Time
	Random  func() (string, error)
}

// WithMCPSignIn wires the browser sign-in. Unwired, the door answers 404.
func (s *Server) WithMCPSignIn(c MCPSignInConfig) *Server {
	s.mcpSignIn = &c
	return s
}

func (s *Server) StartMCPUserSignIn(
	ctx context.Context, req openapi.StartMCPUserSignInRequestObject,
) (openapi.StartMCPUserSignInResponseObject, error) {
	caller, refused := s.mcpCredentialCaller(ctx)
	if refused != nil {
		return openapi.StartMCPUserSignIn403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse: *refused}, nil
	}
	if s.mcpSignIn == nil {
		return openapi.StartMCPUserSignIn404ApplicationProblemPlusJSONResponse(refusal(http.StatusNotFound, CodeNotFound, "Not found", "no browser sign-in is wired on this installation")), nil
	}
	server, err := s.mcpSignIn.Store.MCPSignInServer(ctx, req.Name, caller.ScopesFor(domain.PermToolRead))
	if errors.Is(err, admin.ErrMCPServerUnknown) || errors.Is(err, admin.ErrNoMCPOAuthLogin) {
		return openapi.StartMCPUserSignIn404ApplicationProblemPlusJSONResponse(refusal(http.StatusNotFound, CodeNotFound, "Not found", req.Name)), nil
	}
	if err != nil {
		return nil, err
	}
	authorize, problem, err := s.beginSignIn(ctx, caller.ID, server)
	if problem != nil {
		return openapi.StartMCPUserSignIn400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(*problem),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return openapi.StartMCPUserSignIn200JSONResponse{AuthorizeURL: authorize}, nil
}

// beginSignIn finds where the sign-in goes, records it in flight and answers
// the address the browser opens. A problem is the caller's to fix.
func (s *Server) beginSignIn(
	ctx context.Context, principal domain.UserID, server domain.MCPServer,
) (string, *openapi.Problem, error) {
	cfg, login := s.mcpSignIn, *server.OAuthLogin
	redirect, err := callbackAddress(cfg.BaseURL)
	if err != nil {
		p := invalid(err.Error())
		return "", &p, nil
	}
	endpoints, err := mcpauth.Discover(ctx, cfg.Client(server), server.URL, login)
	if err != nil {
		p := upstreamRefused(signInDiscoveryFailed)
		return "", &p, nil
	}
	if p := endpointsRefused(endpoints, server.Egress); p != nil {
		return "", p, nil
	}
	state, err := cfg.Random()
	if err != nil {
		return "", nil, err
	}
	verifier, err := cfg.Random()
	if err != nil {
		return "", nil, err
	}
	resource := login.Resource
	if resource == "" {
		resource = server.URL
	}
	now := cfg.Now()
	if err := cfg.Store.BeginMCPOAuthFlow(ctx, admin.MCPOAuthFlow{
		State: state, Verifier: verifier, Principal: principal,
		Server: server.Name, ServerScope: server.Scope, ServerAt: server.UpdatedAt,
		ClientID: login.ClientID, TokenAuthMethod: login.TokenAuthMethod, SignInConfig: login.Identity(),
		TokenURL: endpoints.TokenURL, Resource: resource,
		Issuer: endpoints.Issuer, IssParameter: endpoints.IssParameter,
		RedirectURI: redirect, ExpiresAt: now.Add(signInLifetime),
	}, now); err != nil {
		return "", nil, err
	}
	return mcpauth.AuthorizeURL(endpoints.AuthorizeURL, mcpauth.Authorization{
		ClientID: login.ClientID, Scopes: login.Scopes, RedirectURI: redirect,
		State: state, Verifier: verifier, Resource: resource,
	}), nil, nil
}

const signInDiscoveryFailed = "the server's sign-in endpoints could not be found from its metadata; " +
	"name the authorization and token endpoints in its sign-in configuration"

// endpointsRefused holds what the sign-in will reach to the rules a server's
// own address is held to: TLS, unless the endpoint is this machine, and the
// token endpoint inside the server's egress policy, which every refresh of
// the grant will be held to by the worker as well.
func endpointsRefused(e mcpauth.Endpoints, egress *domain.MCPEgress) *openapi.Problem {
	for _, raw := range []string{e.AuthorizeURL, e.TokenURL} {
		if !secureOrLoopback(raw) {
			p := invalid("the sign-in endpoints must be https")
			return &p
		}
	}
	if egress == nil {
		return nil
	}
	u, err := url.Parse(e.TokenURL)
	if err != nil {
		p := invalid("the token endpoint is not an address")
		return &p
	}
	if origin, ok := domain.OriginOf(u); ok && !egress.Allows(u) {
		p := egressNotAllowed(origin)
		return &p
	}
	return nil
}

// callbackAddress is where the browser comes back to: the installation's own
// address, which must be https unless it is this machine, since the code
// travels in it.
func callbackAddress(base string) (string, error) {
	if !secureOrLoopback(base) {
		return "", errors.New("the installation's address must be https for a browser sign-in to come back to it")
	}
	return strings.TrimSuffix(base, "/") + callbackPath, nil
}

func secureOrLoopback(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	host := u.Hostname()
	if host == "localhost" {
		return u.Scheme == "http"
	}
	ip := net.ParseIP(host)
	return u.Scheme == "http" && ip != nil && ip.IsLoopback()
}
