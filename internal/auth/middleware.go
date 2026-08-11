package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// Directory resolves a credential into the principal behind it.
//
// Declared here, by the consumer; the storage implementation lives elsewhere
// and never imports this package.
type Directory interface {
	// PrincipalBySession resolves a live session, refreshing its last-used
	// timestamp. It returns ErrExpired for a session that has lapsed so the
	// caller can tell "signed out" from "never signed in".
	PrincipalBySession(ctx context.Context, tokenHash []byte, now time.Time) (domain.Principal, Session, error)
	// PrincipalByToken resolves a long-lived API token.
	PrincipalByToken(ctx context.Context, tokenHash []byte, now time.Time) (domain.Principal, error)
}

type principalKey struct{}

// WithPrincipal attaches a principal to a context, for tests and for handlers
// that construct one themselves.
func WithPrincipal(ctx context.Context, p domain.Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the authenticated caller.
//
// The bool is not decoration: a handler reached without authentication must
// fail closed rather than operate as a zero-valued principal that happens to
// have no grants.
func PrincipalFrom(ctx context.Context) (domain.Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(domain.Principal)
	return p, ok
}

// Authenticator turns credentials into principals on the way in.
type Authenticator struct {
	dir    Directory
	now    func() time.Time
	secure bool
}

func NewAuthenticator(dir Directory, secure bool, now func() time.Time) *Authenticator {
	if now == nil {
		now = time.Now
	}
	return &Authenticator{dir: dir, now: now, secure: secure}
}

// Middleware authenticates every request and rejects unauthenticated ones.
//
// Authentication is separate from authorisation on purpose: this layer only
// establishes who is calling. What they may do is decided per resource, where
// the scope is known — a single middleware cannot know which company a run
// belongs to before the handler has looked it up.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := a.authenticate(r)
		if err != nil {
			writeProblem(w, statusFor(err), "Not authenticated", err.Error())
			return
		}

		// A cookie-authenticated write must also prove it came from the
		// console; a bearer caller has nothing that rides along automatically.
		if principal.Kind == domain.PrincipalUser {
			if err := CheckCSRF(r); err != nil {
				writeProblem(w, http.StatusForbidden, "Request rejected", err.Error())
				return
			}
		}

		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
	})
}

func (a *Authenticator) authenticate(r *http.Request) (domain.Principal, error) {
	secret, bearer, err := CredentialFrom(r)
	if err != nil {
		return domain.Principal{}, err
	}

	hash := HashToken(secret)
	if bearer {
		return a.dir.PrincipalByToken(r.Context(), hash, a.now())
	}

	principal, _, err := a.dir.PrincipalBySession(r.Context(), hash, a.now())
	return principal, err
}

// Require reports whether the caller may act, and produces the error a handler
// should return when they may not.
//
// It refuses to distinguish "no such resource" from "not allowed to see it":
// answering differently tells an unauthorised caller which runs exist, which
// is a disclosure in itself.
func Require(ctx context.Context, perm domain.Permission, scope domain.Scope) error {
	principal, ok := PrincipalFrom(ctx)
	if !ok {
		return ErrNoCredential
	}
	if !principal.Can(perm, scope) {
		return fmt.Errorf("%w: %s in %s", ErrForbidden, perm, scope)
	}
	return nil
}

// VisibleScopes lists where the caller holds a permission, for filtering a
// listing at the query rather than reading everything and discarding.
func VisibleScopes(ctx context.Context, perm domain.Permission) []domain.Scope {
	principal, ok := PrincipalFrom(ctx)
	if !ok {
		return nil
	}
	return principal.ScopesFor(perm)
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, ErrExpired), errors.Is(err, ErrNoCredential), errors.Is(err, ErrBadCredential):
		return http.StatusUnauthorized
	default:
		return http.StatusUnauthorized
	}
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	// Tell a browser client which scheme to use rather than leaving it to
	// guess from a bare 401.
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Bearer realm="fuseone"`)
	}
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"title":%q,"status":%d,"detail":%q}`, title, status, detail)
}
