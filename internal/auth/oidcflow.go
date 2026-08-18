package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

/*
The sign-in round trip.

State and nonce live in a short cookie rather than in memory, so a console
served by two replicas can still complete a flow the other one started.
*/
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
	return o.StartWithProvider(w, r, p, returnTo)
}

// StartWithProvider begins sign-in using the provider snapshot the caller just
// reconciled.
func (o *OIDC) StartWithProvider(w http.ResponseWriter, r *http.Request, p *OIDCProvider, returnTo string) error {
	if p == nil {
		return ErrNoProvider
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
	return o.CompleteWithProvider(ctx, w, r, p)
}

// CompleteWithProvider verifies the callback using the provider snapshot the
// sign-in route reconciled for this request.
func (o *OIDC) CompleteWithProvider(ctx context.Context, w http.ResponseWriter, r *http.Request, p *OIDCProvider) (Identity, error) {
	if p == nil {
		return Identity{}, ErrNoProvider
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
