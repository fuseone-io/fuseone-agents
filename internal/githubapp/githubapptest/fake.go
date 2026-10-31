// Package githubapptest is a fake GitHub that mints installation tokens the
// way github.com does, for tests on both sides of the worker: the minting code
// and the MCP transport that sends what was minted.
//
// It verifies what GitHub verifies — the JWT's RS256 signature against the
// App's public key, its issuer, and that it is not expired or too long-lived —
// so a test that passes here cannot pass with a JWT GitHub would refuse.
package githubapptest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Claims are the registered claims a GitHub App JWT carries.
type Claims struct {
	Issuer    string `json:"iss"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// Fake is one GitHub App with one installation.
type Fake struct {
	Server         *httptest.Server
	Key            *rsa.PrivateKey
	AppID          int64
	InstallationID int64
	// TTL is how long a minted token lives. GitHub's is one hour.
	TTL time.Duration
	// Prefix is the API root's path, "" for github.com and "/api/v3" for
	// GitHub Enterprise Server.
	Prefix string

	mu      sync.Mutex
	now     func() time.Time
	refuse  int
	tokens  map[string]time.Time
	jwts    []string
	claims  []Claims
	repos   [][]string
	counter int
}

// New starts a fake GitHub on a loopback address with a fresh 2048-bit key.
func New(t testing.TB, appID, installationID int64) *Fake {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate App key: %v", err)
	}
	f := &Fake{
		Key: key, AppID: appID, InstallationID: installationID, TTL: time.Hour,
		now: time.Now, tokens: map[string]time.Time{},
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Server.Close)
	return f
}

// UseClock makes the fake read time from the test's clock, so expiry and
// JWT lifetimes are judged on the same timeline the code under test uses.
func (f *Fake) UseClock(now func() time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = now
}

// Base is the API root to configure as the App's API base URL.
func (f *Fake) Base() string { return f.Server.URL + f.Prefix }

// PKCS1 is the App's private key as GitHub hands it out.
func (f *Fake) PKCS1() string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(f.Key),
	}))
}

// PKCS8 is the same key re-encoded, as `openssl pkcs8 -topk8` leaves it.
func (f *Fake) PKCS8(t testing.TB) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(f.Key)
	if err != nil {
		t.Fatalf("marshal PKCS#8: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// Refuse makes every later mint answer status, the way GitHub answers a
// revoked key (401) or a removed installation (404). Zero mints again.
func (f *Fake) Refuse(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refuse = status
}

// Minted is how many installation tokens were issued.
func (f *Fake) Minted() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.claims)
}

// JWTs are every App JWT presented, accepted or not.
func (f *Fake) JWTs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.jwts...)
}

// LastClaims are the claims of the last accepted JWT.
func (f *Fake) LastClaims() Claims {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.claims) == 0 {
		return Claims{}
	}
	return f.claims[len(f.claims)-1]
}

// LastRepositories is the repository list of the last accepted request.
func (f *Fake) LastRepositories() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.repos) == 0 {
		return nil
	}
	return f.repos[len(f.repos)-1]
}

// Valid reports whether token is an installation token this fake issued and
// that has not expired, which is what an MCP server behind GitHub checks.
func (f *Fake) Valid(token string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	expires, ok := f.tokens[token]
	return ok && f.now().Before(expires)
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	want := fmt.Sprintf("%s/app/installations/%d/access_tokens", f.Prefix, f.InstallationID)
	jwt, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")

	f.mu.Lock()
	defer f.mu.Unlock()
	f.jwts = append(f.jwts, jwt)
	switch {
	case f.refuse != 0:
		http.Error(w, `{"message":"refused"}`, f.refuse)
		return
	case r.URL.Path != want || r.Method != http.MethodPost:
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		return
	}
	claims, err := f.verify(jwt)
	if err != nil {
		http.Error(w, `{"message":"`+err.Error()+`"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		Repositories []string `json:"repositories"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad body"}`, http.StatusUnprocessableEntity)
			return
		}
	}
	f.counter++
	token := "ghs_fake_" + strconv.Itoa(f.counter)
	expires := f.now().Add(f.TTL).UTC().Truncate(time.Second)
	f.tokens[token] = expires
	f.claims = append(f.claims, claims)
	f.repos = append(f.repos, body.Repositories)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token": token, "expires_at": expires.Format(time.RFC3339),
	})
}

// verify checks what GitHub checks. Called with f.mu held.
func (f *Fake) verify(jwt string) (Claims, error) {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return Claims{}, fmt.Errorf("not a JWT")
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if err := decodeSegment(parts[0], &header); err != nil || header.Alg != "RS256" {
		return Claims{}, fmt.Errorf("header is not RS256")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, fmt.Errorf("signature is not base64url")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&f.Key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		return Claims{}, fmt.Errorf("signature does not verify")
	}
	var claims Claims
	if err := decodeSegment(parts[1], &claims); err != nil {
		return Claims{}, fmt.Errorf("claims unreadable")
	}
	now := f.now().Unix()
	switch {
	case claims.Issuer != strconv.FormatInt(f.AppID, 10):
		return Claims{}, fmt.Errorf("issuer is not the App")
	case claims.IssuedAt > now:
		return Claims{}, fmt.Errorf("issued in the future")
	case claims.ExpiresAt <= now:
		return Claims{}, fmt.Errorf("expired")
	case claims.ExpiresAt-claims.IssuedAt > 600:
		return Claims{}, fmt.Errorf("lifetime over ten minutes")
	}
	return claims, nil
}

// decodeSegment takes `any` because it is json.Unmarshal's target: the header
// and the claims are two different shapes decoded the same way.
func decodeSegment(segment string, into any) error {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, into)
}
