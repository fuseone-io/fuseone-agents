package mcpauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/fuseone/agents/internal/domain"
)

// Endpoints is where a sign-in goes. Issuer is the authorization server the
// endpoints were found at, empty when both were configured and nothing was
// fetched; IssParameter is that server saying it names itself in its answer
// (RFC 9207), so the callback can refuse an answer from any other.
type Endpoints struct {
	Issuer       string
	AuthorizeURL string
	TokenURL     string
	IssParameter bool
}

// ErrDiscovery is a server whose sign-in endpoints could not be found. Its
// reason is ours: what the server published is not repeated.
var ErrDiscovery = errors.New("mcpauth: the sign-in endpoints could not be found")

/*
Discover is where the sign-in for the server at serverURL goes. What the
configuration names wins; whatever it leaves out is found from the server's
protected resource metadata and then its authorization server's, through
the MCP SDK's readers, which hold each document to its resource and issuer,
refuse an authorization server without PKCE and refuse dangerous schemes.
*/
func Discover(ctx context.Context, hc *http.Client, serverURL string, login domain.MCPOAuthLogin) (Endpoints, error) {
	if login.AuthorizeURL != "" && login.TokenURL != "" {
		return Endpoints{AuthorizeURL: login.AuthorizeURL, TokenURL: login.TokenURL}, nil
	}
	issuer, err := authorizationServer(ctx, hc, serverURL, login.Resource)
	if err != nil {
		return Endpoints{}, err
	}
	meta, err := serverMetadata(ctx, hc, issuer)
	if err != nil {
		return Endpoints{}, err
	}
	found := Endpoints{
		Issuer: meta.Issuer, AuthorizeURL: meta.AuthorizationEndpoint, TokenURL: meta.TokenEndpoint,
		IssParameter: meta.AuthorizationResponseIssParameterSupported,
	}
	if login.AuthorizeURL != "" {
		found.AuthorizeURL = login.AuthorizeURL
	}
	if login.TokenURL != "" {
		found.TokenURL = login.TokenURL
	}
	return found, nil
}

// authorizationServer is the first authorization server the server's
// protected resource metadata names (RFC 9728 §3.1). The document at the
// well-known path with the server's path inserted describes the server, and is
// held to its address, or to the configured resource; the document at the
// origin's well-known address describes the origin, and is held to that.
func authorizationServer(ctx context.Context, hc *http.Client, serverURL, configured string) (string, error) {
	u, err := url.Parse(serverURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("%w: the server's address is not an address", ErrDiscovery)
	}
	for _, c := range resourceDocuments(u, configured) {
		prm, err := oauthex.GetProtectedResourceMetadata(ctx, c.metadata, c.resource, hc)
		if err == nil && prm != nil && len(prm.AuthorizationServers) > 0 {
			return prm.AuthorizationServers[0], nil
		}
	}
	return "", fmt.Errorf("%w: the server publishes no protected resource metadata naming an authorization server", ErrDiscovery)
}

// resourceDocument is where a protected resource's metadata may be, and the
// resource that document must describe to be this server's.
type resourceDocument struct {
	metadata, resource string
}

func resourceDocuments(u *url.URL, configured string) []resourceDocument {
	origin := u.Scheme + "://" + u.Host
	path := strings.TrimSuffix(u.Path, "/")
	at := origin + "/.well-known/oauth-protected-resource"
	if path == "" {
		resource := configured
		if resource == "" {
			resource = u.String()
		}
		return []resourceDocument{{at, resource}}
	}
	own := configured
	if own == "" {
		own = u.String()
	}
	out := []resourceDocument{{at + path, own}}
	if configured != "" {
		// A resource the administrator named is the one any document must
		// describe, wherever it is published.
		return append(out, resourceDocument{at, configured})
	}
	// The origin's document names the origin, with or without the slash.
	return append(out, resourceDocument{at, origin}, resourceDocument{at, origin + "/"})
}

// serverMetadata is the authorization server's metadata: RFC 8414 with the
// issuer's path inserted, then OpenID discovery inserted and appended, the
// last being where an issuer with a path, such as a realm, keeps it.
func serverMetadata(ctx context.Context, hc *http.Client, issuer string) (*oauthex.AuthServerMeta, error) {
	u, err := url.Parse(issuer)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("%w: the authorization server named is not an address", ErrDiscovery)
	}
	candidates := wellKnown(u, "oauth-authorization-server")
	candidates = append(candidates, wellKnown(u, "openid-configuration")...)
	if path := strings.TrimSuffix(u.Path, "/"); path != "" {
		candidates = append(candidates, u.Scheme+"://"+u.Host+path+"/.well-known/openid-configuration")
	}
	for _, at := range candidates {
		meta, err := oauthex.GetAuthServerMeta(ctx, at, issuer, hc)
		if err == nil && meta != nil {
			return meta, nil
		}
	}
	return nil, fmt.Errorf("%w: the authorization server publishes no usable metadata", ErrDiscovery)
}

// wellKnown is the document's address with the path inserted, then at the
// origin alone when there is a path.
func wellKnown(u *url.URL, document string) []string {
	origin := u.Scheme + "://" + u.Host + "/.well-known/" + document
	path := strings.TrimSuffix(u.Path, "/")
	if path == "" {
		return []string{origin}
	}
	return []string{origin + path, origin}
}

// Authorization is what the browser is sent to sign in with.
type Authorization struct {
	ClientID    string
	Scopes      []string
	RedirectURI string
	State       string
	// Verifier is the PKCE verifier; only its S256 challenge leaves.
	Verifier string
	Resource string
}

// AuthorizeURL is the address the browser opens: the endpoint, with any query
// of its own kept, asking for a code under PKCE S256 for the resource.
func AuthorizeURL(endpoint string, a Authorization) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", a.ClientID)
	q.Set("redirect_uri", a.RedirectURI)
	q.Set("state", a.State)
	q.Set("code_challenge", Challenge(a.Verifier))
	q.Set("code_challenge_method", "S256")
	if len(a.Scopes) > 0 {
		q.Set("scope", strings.Join(a.Scopes, " "))
	}
	if a.Resource != "" {
		q.Set("resource", a.Resource)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// Challenge is a PKCE verifier's S256 challenge (RFC 7636 §4.2).
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
