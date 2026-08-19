package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

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
	// Revision names the durable configuration this live provider represents.
	// It lets a serve replica avoid rediscovering an unchanged provider on the
	// public sign-in path while still noticing a mapping saved by another
	// replica.
	Revision string
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
	mu          sync.RWMutex
	providers   map[string]*OIDCProvider
	discoveryMu sync.Mutex
	discovered  map[string]*oidc.Provider
	baseURL     string
	secure      bool
}

// lookup returns a configured provider by id.
func (o *OIDC) lookup(id string) (*OIDCProvider, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	p, ok := o.providers[id]
	if !ok {
		return nil, false
	}
	return cloneOIDCProvider(p), true
}

func NewOIDC(baseURL string, secure bool) *OIDC {
	return &OIDC{
		providers:  make(map[string]*OIDCProvider),
		discovered: make(map[string]*oidc.Provider),
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		secure:     secure,
	}
}

// IdentityProviderRevision turns the durable provider row into a cheap change
// detector for the per-process sign-in registry.
func IdentityProviderRevision(p domain.IdentityProvider) string {
	if !p.UpdatedAt.IsZero() {
		return "updated_at:" + p.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000000000Z07:00")
	}
	// Tests and bootstrap-only stores may not carry timestamps. Fall back to a
	// content digest so the invariant still holds there without inventing a
	// database-only requirement for callers.
	body, _ := json.Marshal(struct {
		ID          string                `json:"id"`
		Display     string                `json:"display"`
		Issuer      string                `json:"issuer"`
		ClientID    string                `json:"clientId"`
		GroupsClaim string                `json:"groupsClaim"`
		Mappings    []domain.GroupMapping `json:"mappings"`
		Enabled     bool                  `json:"enabled"`
	}{
		ID: p.ID, Display: p.Display, Issuer: p.Issuer, ClientID: p.ClientID,
		GroupsClaim: p.GroupsClaim, Mappings: p.Mappings, Enabled: p.Enabled,
	})
	sum := sha256.Sum256(body)
	return fmt.Sprintf("definition:%x", sum[:])
}

func cloneOIDCProvider(p *OIDCProvider) *OIDCProvider {
	if p == nil {
		return nil
	}
	out := *p
	out.Mappings = slices.Clone(p.Mappings)
	return &out
}

func (o *OIDC) discover(ctx context.Context, issuer string) (*oidc.Provider, error) {
	o.discoveryMu.Lock()
	defer o.discoveryMu.Unlock()
	if found := o.discovered[issuer]; found != nil {
		return found, nil
	}
	discovered, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, err
	}
	o.discovered[issuer] = discovered
	return discovered, nil
}

// Add discovers a provider's endpoints and registers it.
//
// Discovery happens at registration rather than per sign-in: an unreachable
// provider should fail when an operator configures it, not for the first
// person who tries to sign in on Monday morning.
func (o *OIDC) Add(ctx context.Context, p *OIDCProvider) error {
	discovered, err := o.discover(ctx, p.Issuer)
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
	o.providers[p.ID] = cloneOIDCProvider(p)
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
		out = append(out, cloneOIDCProvider(p))
	}
	slices.SortFunc(out, func(a, b *OIDCProvider) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// Provider returns the currently live provider by id.
func (o *OIDC) Provider(id string) (*OIDCProvider, bool) {
	return o.lookup(id)
}

// flowCookie carries the one-time values that tie a callback to the browser
// that started the flow.
const flowCookie = "fuseone_oidc_flow"

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
		if err != nil ||
			m.Company == "" ||
			(m.Company == string(domain.Installation) && m.Area != "") ||
			(m.Company != string(domain.Installation) && m.Area == "") {
			// A malformed mapping grants nothing rather than something
			// approximate. Silently widening a company scope here would be
			// invisible; the only empty area accepted is the explicit
			// installation scope, written as company "*".
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
