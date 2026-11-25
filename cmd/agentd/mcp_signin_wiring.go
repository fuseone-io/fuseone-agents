package main

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi"
)

// mcpSignIn wires a person signing in to a remote MCP server from their
// browser: the store, the installation's address the browser comes back to,
// and the outbound client a sign-in reaches the server and its authorization
// server through.
func mcpSignIn(integrations *admin.Integrations, baseURL string) httpapi.MCPSignInConfig {
	return httpapi.MCPSignInConfig{
		Store:   integrations,
		Client:  signInHTTPClient,
		BaseURL: baseURL,
		Now:     time.Now,
		Random:  signInRandom,
	}
}

// signInHTTPClient is the worker's own transport for the server, with its egress
// policy: discovery and the code exchange are requests of the server's, held
// as every call and every refresh of the grant will be.
func signInHTTPClient(server domain.MCPServer) *http.Client {
	return &http.Client{
		Transport: guardEgress(server.Name, effectiveEgress(server), baseHTTPTransport()),
		Timeout:   15 * time.Second,
	}
}

// signInRandom is a state or a PKCE verifier: 32 bytes from the system's
// source, generated here at the edge and recorded with the sign-in.
func signInRandom() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// mountMCPSignInReturn serves where the browser comes back from a server's
// sign-in: outside the API's contract, as a sign-in's return is, and inside
// the session, since who returns is judged against who began.
func mountMCPSignInReturn(root *http.ServeMux, api *httpapi.Server, session func(http.Handler) http.Handler) {
	root.Handle("GET /auth/mcp/callback", session(api.SignInCallback()))
}
