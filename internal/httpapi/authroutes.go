package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
)

// AuthRoutes serves sign-in, sign-out and first-run setup.
//
// These live outside the OpenAPI contract on purpose: they are browser
// redirects and cookie handling, not an API a CLI or CI job would call. The
// contract describes what a machine can do; this describes how a person
// arrives.
type AuthRoutes struct {
	oidc      *auth.OIDC
	dir       *auth.Postgres
	bootstrap *auth.Bootstrap
	secure    bool
}

func NewAuthRoutes(oidc *auth.OIDC, dir *auth.Postgres, bootstrap *auth.Bootstrap, secure bool) *AuthRoutes {
	return &AuthRoutes{oidc: oidc, dir: dir, bootstrap: bootstrap, secure: secure}
}

// Mount registers the routes. Everything here is deliberately unauthenticated:
// it is how a caller becomes authenticated in the first place.
func (a *AuthRoutes) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/providers", a.providers)
	mux.HandleFunc("GET /auth/start/{provider}", a.start)
	mux.HandleFunc("GET /auth/callback/{provider}", a.callback)
	mux.HandleFunc("POST /auth/logout", a.logout)
	mux.HandleFunc("POST /auth/bootstrap", a.claimBootstrap)
}

// providers tells the sign-in page what to offer.
//
// It also reports whether setup is still pending, so a fresh installation
// shows the setup screen instead of a login form with no providers on it.
func (a *AuthRoutes) providers(w http.ResponseWriter, r *http.Request) {
	type option struct {
		ID      string `json:"id"`
		Display string `json:"display"`
	}
	out := struct {
		Providers        []option `json:"providers"`
		BootstrapPending bool     `json:"bootstrapPending"`
		// AuthRequired tells the console whether there is anything to sign in
		// to. An installation with no identity configured is not protecting
		// anything, and a sign-in screen in front of it is a lock on an open
		// door — it stops nobody and confuses everybody.
		AuthRequired bool `json:"authRequired"`
	}{Providers: []option{}, AuthRequired: true}

	for _, p := range a.oidc.Providers() {
		out.Providers = append(out.Providers, option{ID: p.ID, Display: p.Display})
	}

	if a.bootstrap != nil {
		// Whether a token can be claimed, not whether an administrator is
		// missing. An installation reopened after a lockout has both an
		// administrator and a live token, and it has to show the setup form
		// or the operator holding that token has nowhere to type it.
		open, err := a.bootstrap.Open(r.Context())
		if err != nil {
			writeProblemJSON(w, http.StatusInternalServerError, "Could not read setup state", err.Error())
			return
		}
		out.BootstrapPending = open
	}

	writeJSON(w, http.StatusOK, out)
}

func (a *AuthRoutes) start(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if err := a.oidc.Start(w, r, provider, r.URL.Query().Get("returnTo")); err != nil {
		writeProblemJSON(w, http.StatusBadRequest, "Cannot start sign-in", err.Error())
	}
}

// callback completes sign-in and issues a session.
func (a *AuthRoutes) callback(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("provider")

	identity, err := a.oidc.Complete(r.Context(), w, r, providerID)
	if err != nil {
		writeProblemJSON(w, http.StatusUnauthorized, "Sign-in failed", err.Error())
		return
	}

	var provider *auth.OIDCProvider
	for _, p := range a.oidc.Providers() {
		if p.ID == providerID {
			provider = p
			break
		}
	}
	if provider == nil {
		writeProblemJSON(w, http.StatusBadRequest, "Unknown provider", providerID)
		return
	}

	principalID, err := a.dir.UpsertPrincipal(r.Context(),
		identity.Provider, identity.Subject, identity.Display, identity.Email)
	if err != nil {
		writeProblemJSON(w, http.StatusInternalServerError, "Could not record the sign-in", err.Error())
		return
	}

	// Grants are replaced, not merged: somebody removed from a group loses the
	// access on their next sign-in rather than keeping it for ever.
	grants := provider.GrantsFor(identity.Groups)
	if err := a.dir.ReplaceAssertedGrants(r.Context(), principalID, providerID, grants); err != nil {
		writeProblemJSON(w, http.StatusInternalServerError, "Could not apply access", err.Error())
		return
	}

	if len(grants) == 0 {
		// Authenticated but granted nothing. Saying so plainly is far kinder
		// than an empty console the person cannot explain.
		writeProblemJSON(w, http.StatusForbidden, "No access granted",
			"Your sign-in worked, but none of your groups map to a role. Ask an administrator to map "+
				strings.Join(identity.Groups, ", ")+" to a company, area and role.")
		return
	}

	if err := a.issueSession(r.Context(), w, r, principalID); err != nil {
		writeProblemJSON(w, http.StatusInternalServerError, "Could not start the session", err.Error())
		return
	}

	http.Redirect(w, r, identity.ReturnTo, http.StatusFound)
}

// claimBootstrap exchanges the first-run token for the first administrator.
func (a *AuthRoutes) claimBootstrap(w http.ResponseWriter, r *http.Request) {
	if a.bootstrap == nil {
		writeProblemJSON(w, http.StatusGone, "Setup is not available", "this installation has no setup path")
		return
	}

	var body struct {
		Token   string `json:"token"`
		Display string `json:"display"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		writeProblemJSON(w, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}

	session, principal, err := a.bootstrap.Claim(r.Context(),
		body.Token, body.Display, r.UserAgent(), clientIP(r))
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, auth.ErrBootstrapClosed) {
			// Gone rather than Forbidden: the path existed and is now closed
			// permanently, which is what the caller needs to know.
			status = http.StatusGone
		}
		writeProblemJSON(w, status, "Setup failed", err.Error())
		return
	}

	expires := time.Now().Add(auth.SessionTTL)
	auth.SetCookie(w, session.Secret, expires, a.secure)
	if err := a.issueCSRF(w, expires); err != nil {
		writeProblemJSON(w, http.StatusInternalServerError, "Could not start the session", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, meFrom(principal))
}

func (a *AuthRoutes) logout(w http.ResponseWriter, r *http.Request) {
	if secret, _, err := auth.CredentialFrom(r); err == nil {
		// Revoking server-side is what makes sign-out mean something: clearing
		// the cookie alone leaves a credential that still works if it was
		// copied anywhere.
		_ = a.dir.RevokeSession(r.Context(), auth.HashToken(secret), time.Now())
	}
	auth.ClearCookie(w, a.secure)
	auth.SetCSRFCookie(w, "", time.Unix(0, 0), a.secure)
	w.WriteHeader(http.StatusNoContent)
}

func (a *AuthRoutes) issueSession(ctx context.Context, w http.ResponseWriter, r *http.Request, principalID string) error {
	token, session, err := a.dir.CreateSession(ctx, principalID, r.UserAgent(), clientIP(r), time.Now())
	if err != nil {
		return err
	}
	auth.SetCookie(w, token.Secret, session.ExpiresAt, a.secure)
	return a.issueCSRF(w, session.ExpiresAt)
}

// issueCSRF sets the double-submit value the console echoes back on writes.
func (a *AuthRoutes) issueCSRF(w http.ResponseWriter, expires time.Time) error {
	token, err := auth.NewToken()
	if err != nil {
		return err
	}
	auth.SetCSRFCookie(w, token.Secret, expires, a.secure)
	return nil
}

// Me describes the signed-in caller to the console.
type Me struct {
	ID      string    `json:"id"`
	Display string    `json:"display"`
	Kind    string    `json:"kind"`
	Grants  []MeGrant `json:"grants"`
	Can     []string  `json:"can"`
}

type MeGrant struct {
	Company string `json:"company"`
	Area    string `json:"area"`
	Role    string `json:"role"`
}

// meFrom renders the caller, including the permissions they hold anywhere.
//
// The console uses `can` to decide which navigation to show. It is a hint for
// the interface, never the enforcement — every request is checked again on the
// server, where the scope of the specific resource is known.
func meFrom(p domain.Principal) Me {
	out := Me{
		ID: string(p.ID), Display: p.Display, Kind: string(p.Kind),
		Grants: []MeGrant{}, Can: []string{},
	}
	seen := map[domain.Permission]struct{}{}

	for _, g := range p.Grants {
		out.Grants = append(out.Grants, MeGrant{
			Company: string(g.Scope.Company), Area: string(g.Scope.Area), Role: string(g.Role),
		})
		for _, perm := range g.Role.Permissions() {
			if _, dup := seen[perm]; !dup {
				seen[perm] = struct{}{}
				out.Can = append(out.Can, string(perm))
			}
		}
	}
	return out
}

// MeHandler answers "who am I", for the console's first request after load.
func MeHandler(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		writeProblemJSON(w, http.StatusUnauthorized, "Not signed in", "no session")
		return
	}
	writeJSON(w, http.StatusOK, meFrom(principal))
}

// clientIP prefers the proxy's forwarded address, which is what an operator
// sees in a session list when checking whether a sign-in was theirs.
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if first, _, found := strings.Cut(fwd, ","); found {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(fwd)
	}
	host, _, found := strings.Cut(r.RemoteAddr, ":")
	if !found {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeProblemJSON(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"title": title, "status": status, "detail": detail,
	})
}

// OpenInstallation answers for a server running with no identity at all.
//
// It says so plainly rather than pretending: the console renders, and the
// process log has already warned that every caller has full access.
func OpenInstallation(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"providers":        []any{},
		"bootstrapPending": false,
		"authRequired":     false,
	})
}
