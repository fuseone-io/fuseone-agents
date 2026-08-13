package auth

import (
	"context"
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
