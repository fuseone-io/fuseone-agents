package guard

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Clock is injected so token expiry is testable without waiting an hour.
type Clock interface{ Now() time.Time }

// SystemClock is the wall clock, for main.
type SystemClock struct{}

// Now returns the current time.
func (SystemClock) Now() time.Time { return time.Now() }

// refreshMargin is how long before expiry a token is replaced. One call reads
// half a dozen endpoints and then merges; it must not run out half-way.
const refreshMargin = 5 * time.Minute

// AppTokenOptions configure an AppTokenSource.
type AppTokenOptions struct {
	Credentials AppCredentials
	APIURL      string
	Repo        Repo
	HTTP        *http.Client
	Clock       Clock
}

// AppTokenSource mints GitHub App installation tokens and caches each until
// shortly before it expires.
//
// App credentials rather than a pre-minted token: an installation token lives
// one hour, and a server that runs for days cannot hold one. And an App rather
// than a personal token is mandatory for QA, because the ruleset (#207) pins
// factory/qa to the QA App's integration id, and a status is attributable to
// an App only when that App's installation token created it.
type AppTokenSource struct {
	opts AppTokenOptions
	key  *rsa.PrivateKey

	mu      sync.Mutex
	token   string
	expires time.Time
}

// NewAppTokenSource parses the private key up front, so a bad key stops the
// server at start rather than at the first merge.
func NewAppTokenSource(opts AppTokenOptions) (*AppTokenSource, error) {
	key, err := parseRSAKey(opts.Credentials.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}
	if opts.HTTP == nil {
		opts.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	if opts.Clock == nil {
		opts.Clock = SystemClock{}
	}
	return &AppTokenSource{opts: opts, key: key}, nil
}

func parseRSAKey(raw []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("%w: private key is not PEM", ErrConfig)
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: private key: %w", ErrConfig, err)
	}
	k, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: private key is not RSA", ErrConfig)
	}
	return k, nil
}

// Token returns a valid installation token, minting one when needed.
func (s *AppTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.opts.Clock.Now()
	if s.token != "" && now.Before(s.expires.Add(-refreshMargin)) {
		return s.token, nil
	}
	token, expires, err := s.mint(ctx, now)
	if err != nil {
		return "", err
	}
	s.token, s.expires = token, expires
	return token, nil
}

// mint exchanges a JWT for an installation token restricted to the one
// repository, so even a leaked token reaches nothing else.
func (s *AppTokenSource) mint(ctx context.Context, now time.Time) (string, time.Time, error) {
	jwt, err := s.appJWT(now)
	if err != nil {
		return "", time.Time{}, err
	}
	body, _ := json.Marshal(map[string][]string{"repositories": {s.opts.Repo.Name}})
	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", s.opts.APIURL, s.opts.Credentials.InstallationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("github app token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := s.opts.HTTP.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("github app token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		return "", time.Time{}, fmt.Errorf("%w: app token: status %d", ErrGitHub, resp.StatusCode)
	}
	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Token == "" {
		return "", time.Time{}, errors.Join(fmt.Errorf("%w: app token: unreadable response", ErrGitHub), err)
	}
	return out.Token, out.ExpiresAt, nil
}

// appJWT signs the App's RS256 assertion. iat is backdated a minute for clock
// drift, exp kept under GitHub's ten-minute ceiling. Hand-rolled because it is
// a dozen lines of stdlib, not worth a dependency.
func (s *AppTokenSource) appJWT(now time.Time) (string, error) {
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{
		"iat": now.Add(-time.Minute).Unix(),
		"exp": now.Add(8 * time.Minute).Unix(),
		"iss": strconv.FormatInt(s.opts.Credentials.AppID, 10),
	})
	signing := header + "." + enc.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("github app jwt: %w", err)
	}
	return signing + "." + enc.EncodeToString(sig), nil
}
