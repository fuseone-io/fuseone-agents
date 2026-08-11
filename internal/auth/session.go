// Package auth authenticates callers and enforces what they may do.
//
// The platform never stores a password (PRD DE-04). People arrive through the
// installation's own identity provider; machines arrive with a token an
// operator issued. Both end up as a domain.Principal carrying scoped grants,
// and every check downstream asks the same question of it.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var (
	ErrNoCredential  = errors.New("auth: no credential presented")
	ErrBadCredential = errors.New("auth: credential is not valid")
	ErrExpired       = errors.New("auth: credential has expired")
	ErrForbidden     = errors.New("auth: not permitted in this scope")
)

// CookieName holds the session identifier in the browser.
const CookieName = "fuseone_session"

// CSRFHeader is the header a state-changing request must echo.
//
// Cookies ride along automatically on cross-site requests, so the cookie alone
// authenticates but does not authorise a write. The double-submit check below
// is what closes that gap.
const CSRFHeader = "X-CSRF-Token"

// tokenBytes is the entropy in a session or API token. 32 bytes is well past
// what a brute force can reach and short enough to fit a cookie comfortably.
const tokenBytes = 32

// Token is a credential in two halves: the secret handed to the client, and
// the hash kept in the database.
//
// Only the hash is ever stored. A stolen database backup must not be a pile of
// usable credentials, and the difference costs one line at issue time.
type Token struct {
	Secret string
	Hash   []byte
}

// NewToken mints a credential.
func NewToken() (Token, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return Token{}, fmt.Errorf("auth: generate token: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	return Token{Secret: secret, Hash: HashToken(secret)}, nil
}

// HashToken derives the stored form of a credential.
//
// A plain SHA-256 is right here and a password hash would be wrong: the input
// is 256 bits of machine-generated entropy, so there is no dictionary to
// stretch against, and a slow hash on every request would be a self-inflicted
// denial of service.
func HashToken(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// EqualHash compares in constant time.
func EqualHash(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// Session is a signed-in principal.
type Session struct {
	ID          string
	PrincipalID string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	RevokedAt   time.Time
}

func (s Session) Active(now time.Time) bool {
	return s.RevokedAt.IsZero() && now.Before(s.ExpiresAt)
}

// SetCookie writes the session cookie.
//
// httpOnly keeps the token out of reach of any script on the page, so an XSS
// bug cannot walk off with a credential that approves financial actions.
// SameSite=Lax lets an ordinary link into the console still arrive signed in
// while blocking the cross-site form posts CSRF depends on.
func SetCookie(w http.ResponseWriter, secret string, expires time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    secret,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearCookie removes the session cookie on sign-out.
func ClearCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// CredentialFrom extracts whatever credential a request carries.
//
// A bearer token wins over a cookie: a machine caller that also happens to
// hold a browser session should be treated as the machine it claims to be.
func CredentialFrom(r *http.Request) (secret string, bearer bool, err error) {
	if header := r.Header.Get("Authorization"); header != "" {
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || strings.TrimSpace(token) == "" {
			return "", false, ErrBadCredential
		}
		return strings.TrimSpace(token), true, nil
	}

	cookie, err := r.Cookie(CookieName)
	if err != nil || cookie.Value == "" {
		return "", false, ErrNoCredential
	}
	return cookie.Value, false, nil
}

// CheckCSRF verifies a cookie-authenticated write.
//
// The double-submit pattern: the client reads the CSRF cookie with JavaScript
// and echoes it in a header. A cross-site attacker can cause the browser to
// send the cookie but cannot read it to set the header, so the two matching
// proves the request came from the console rather than from someone else's
// page. Bearer callers are exempt — nothing attaches an Authorization header
// automatically, so there is no confused deputy to protect against.
func CheckCSRF(r *http.Request) error {
	if !mutating(r.Method) {
		return nil
	}

	header := r.Header.Get(CSRFHeader)
	if header == "" {
		return fmt.Errorf("%w: missing %s", ErrBadCredential, CSRFHeader)
	}
	cookie, err := r.Cookie(CSRFCookieName)
	if err != nil || cookie.Value == "" {
		return fmt.Errorf("%w: missing CSRF cookie", ErrBadCredential)
	}
	if subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) != 1 {
		return fmt.Errorf("%w: CSRF token mismatch", ErrBadCredential)
	}
	return nil
}

// CSRFCookieName is readable by the console's own scripts, unlike the session
// cookie. It carries no authority on its own — it only proves the request was
// composed by code running on this origin.
const CSRFCookieName = "fuseone_csrf"

// SetCSRFCookie issues the double-submit value.
func SetCSRFCookie(w http.ResponseWriter, value string, expires time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: false, // deliberately readable; that is the whole mechanism
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func mutating(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}
