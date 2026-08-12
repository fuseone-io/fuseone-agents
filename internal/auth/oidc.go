package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/fuseone/agents/internal/domain"
)

var ErrNoProvider = errors.New("auth: no such identity provider")

// GroupMapping turns an assertion's group into scoped grants.
//
// Without at least one mapping a successful sign-in grants nothing. That is
// the correct default: authenticating proves who someone is, and it should
// never by itself decide what they may do.
//
// The type lives in domain because the administration area writes these and
// the sign-in flow reads them; two declarations of the same four fields is how
// the screen and the flow end up disagreeing about what a mapping is.
type GroupMapping = domain.GroupMapping

// OIDCProvider is one configured identity provider.
type OIDCProvider struct {
	ID       string
	Display  string
	Issuer   string
	ClientID string
	// ClientSecret comes from the vault, never from the provider record.
	ClientSecret string
	// GroupsClaim names the claim carrying group membership. Providers differ:
	// Keycloak and Okta commonly use "groups", Entra ID "roles".
	GroupsClaim string
	Mappings    []domain.GroupMapping

	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
}

// OIDC runs the browser sign-in flow.
type OIDC struct {
	// mu guards providers. The administration area writes this map while
	// sign-in requests read it: configuring a provider is no longer something
	// that only happens before the server starts serving.
	mu        sync.RWMutex
	providers map[string]*OIDCProvider
	baseURL   string
	secure    bool
}

// lookup returns a configured provider by id.
func (o *OIDC) lookup(id string) (*OIDCProvider, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	p, ok := o.providers[id]
	return p, ok
}

func NewOIDC(baseURL string, secure bool) *OIDC {
	return &OIDC{
		providers: make(map[string]*OIDCProvider),
		baseURL:   strings.TrimSuffix(baseURL, "/"),
		secure:    secure,
	}
}

// Add discovers a provider's endpoints and registers it.
//
// Discovery happens at registration rather than per sign-in: an unreachable
// provider should fail when an operator configures it, not for the first
// person who tries to sign in on Monday morning.
func (o *OIDC) Add(ctx context.Context, p *OIDCProvider) error {
	discovered, err := oidc.NewProvider(ctx, p.Issuer)
	if err != nil {
		return fmt.Errorf("auth: discover %s at %s: %w", p.ID, p.Issuer, err)
	}

	p.verifier = discovered.Verifier(&oidc.Config{ClientID: p.ClientID})
	p.oauth = &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		Endpoint:     discovered.Endpoint(),
		RedirectURL:  o.baseURL + "/auth/callback/" + p.ID,
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	if p.GroupsClaim == "" {
		p.GroupsClaim = "groups"
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	o.providers[p.ID] = p
	return nil
}

// Remove takes a provider out of the live registry.
//
// The sign-in routes read from here rather than from the database, so a
// provider deleted in the administration area has to leave this map too — or
// it keeps accepting sign-ins for a configuration nobody can see any more.
func (o *OIDC) Remove(id string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.providers, id)
}

func (o *OIDC) Providers() []*OIDCProvider {
	o.mu.RLock()
	defer o.mu.RUnlock()

	out := make([]*OIDCProvider, 0, len(o.providers))
	for _, p := range o.providers {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b *OIDCProvider) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// flowCookie carries the one-time values that tie a callback to the browser
// that started the flow.
const flowCookie = "fuseone_oidc_flow"

// Start begins sign-in and redirects the browser to the provider.
//
// Two protections ride in a short-lived cookie. The state value proves the
// callback belongs to a flow this browser started, so an attacker cannot feed
// their own authorization code into someone else's session. The PKCE verifier
// proves the party redeeming the code is the party that requested it, which
// matters because the code travels through the user's browser where an
// intercepting app could otherwise replay it.
func (o *OIDC) Start(w http.ResponseWriter, r *http.Request, providerID, returnTo string) error {
	p, ok := o.lookup(providerID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoProvider, providerID)
	}

	state, err := randomString()
	if err != nil {
		return err
	}
	verifier, err := randomString()
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:  flowCookie,
		Value: state + "|" + verifier + "|" + sanitiseReturn(returnTo),
		Path:  "/auth",
		// Five minutes is generous for a redirect round trip and short enough
		// that an abandoned flow cannot be resumed later.
		Expires:  time.Now().Add(5 * time.Minute),
		HttpOnly: true,
		Secure:   o.secure,
		SameSite: http.SameSiteLaxMode,
	})

	challenge := sha256.Sum256([]byte(verifier))
	url := p.oauth.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:])),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)

	http.Redirect(w, r, url, http.StatusFound)
	return nil
}

// Identity is what a provider asserted about someone.
type Identity struct {
	Provider string
	Subject  string
	Display  string
	Email    string
	Groups   []string
	ReturnTo string
}

// Complete verifies the callback and returns the asserted identity.
func (o *OIDC) Complete(ctx context.Context, w http.ResponseWriter, r *http.Request, providerID string) (Identity, error) {
	p, ok := o.lookup(providerID)
	if !ok {
		return Identity{}, fmt.Errorf("%w: %s", ErrNoProvider, providerID)
	}

	cookie, err := r.Cookie(flowCookie)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: the sign-in flow expired or was not started here", ErrBadCredential)
	}
	// The flow is single use whatever happens next.
	http.SetCookie(w, &http.Cookie{Name: flowCookie, Path: "/auth", MaxAge: -1, HttpOnly: true, Secure: o.secure})

	parts := strings.SplitN(cookie.Value, "|", 3)
	if len(parts) != 3 {
		return Identity{}, fmt.Errorf("%w: malformed sign-in flow", ErrBadCredential)
	}
	state, verifier, returnTo := parts[0], parts[1], parts[2]

	if r.URL.Query().Get("state") != state {
		return Identity{}, fmt.Errorf("%w: state mismatch", ErrBadCredential)
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		// The provider reports a refusal in the query rather than as an error.
		if e := r.URL.Query().Get("error"); e != "" {
			return Identity{}, fmt.Errorf("%w: the provider refused: %s", ErrBadCredential, e)
		}
		return Identity{}, fmt.Errorf("%w: no authorization code", ErrBadCredential)
	}

	tok, err := p.oauth.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("auth: exchange code: %w", err)
	}

	rawID, ok := tok.Extra("id_token").(string)
	if !ok {
		return Identity{}, fmt.Errorf("%w: the provider returned no id token", ErrBadCredential)
	}
	// Verification is what makes the claims trustworthy: signature, issuer,
	// audience and expiry. An unverified id token is just a string the browser
	// handed us.
	idToken, err := p.verifier.Verify(ctx, rawID)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: id token failed verification: %v", ErrBadCredential, err)
	}

	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("auth: read claims: %w", err)
	}

	return Identity{
		Provider: p.ID,
		Subject:  idToken.Subject,
		Display:  firstString(claims, "name", "preferred_username", "email"),
		Email:    firstString(claims, "email"),
		Groups:   stringsFrom(claims[p.GroupsClaim]),
		ReturnTo: returnTo,
	}, nil
}

// GrantsFor turns asserted groups into scoped grants.
//
// Group names are compared case-insensitively but otherwise exactly: no
// prefix or wildcard matching. A wildcard in an access rule is a mistake
// nobody notices until it grants more than intended.
func (p *OIDCProvider) GrantsFor(groups []string) []domain.Grant {
	held := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		held[strings.ToLower(strings.TrimSpace(g))] = struct{}{}
	}

	var out []domain.Grant
	for _, m := range p.Mappings {
		if _, ok := held[strings.ToLower(strings.TrimSpace(m.Group))]; !ok {
			continue
		}
		role, err := domain.ParseRole(m.Role)
		if err != nil || m.Company == "" || m.Area == "" {
			// A malformed mapping grants nothing rather than something
			// approximate. Silently widening a scope here would be invisible.
			continue
		}
		grant := domain.Grant{
			Scope: domain.Scope{Company: domain.CompanyID(m.Company), Area: domain.AreaID(m.Area)},
			Role:  role,
		}
		if !slices.Contains(out, grant) {
			out = append(out, grant)
		}
	}
	return out
}

func randomString() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("auth: random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// sanitiseReturn keeps an open redirect out of the sign-in flow.
//
// Only a same-site path is ever honoured: anything absolute or protocol
// relative would let a crafted sign-in link bounce the operator to an
// attacker's page carrying the appearance of a legitimate login.
func sanitiseReturn(to string) string {
	if to == "" || !strings.HasPrefix(to, "/") || strings.HasPrefix(to, "//") {
		return "/"
	}
	return to
}

func firstString(claims map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := claims[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func stringsFrom(v any) []string {
	switch typed := v.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		// Some providers flatten a single group to a bare string.
		return []string{typed}
	}
	return nil
}
