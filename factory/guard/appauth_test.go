package guard

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// tokenEndpoint stands in for POST /app/installations/{id}/access_tokens. It
// verifies the JWT the way GitHub does: signature, issuer, and a window that
// contains the moment of the call.
type tokenEndpoint struct {
	t     *testing.T
	pub   *rsa.PublicKey
	clock *fakeClock

	mu      sync.Mutex
	mints   int
	scoped  []string
	expires time.Duration
}

func (e *tokenEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/app/installations/456/access_tokens" {
		http.NotFound(w, r)
		return
	}
	claims, ok := e.verify(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if !ok || claims.Iss != "123" {
		http.Error(w, "bad jwt", http.StatusUnauthorized)
		return
	}
	var body struct {
		Repositories []string `json:"repositories"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	e.mu.Lock()
	e.mints++
	e.scoped = body.Repositories
	n := e.mints
	e.mu.Unlock()

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token":      "ghs_minted_" + string(rune('0'+n)),
		"expires_at": e.clock.Now().Add(e.expires).Format(time.RFC3339),
	})
}

type jwtClaims struct {
	Iss string `json:"iss"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

func (e *tokenEndpoint) verify(token string) (jwtClaims, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return jwtClaims{}, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return jwtClaims{}, false
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(e.pub, crypto.SHA256, digest[:], sig) != nil {
		return jwtClaims{}, false
	}
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var c jwtClaims
	if json.Unmarshal(raw, &c) != nil {
		return jwtClaims{}, false
	}
	now := e.clock.Now().Unix()
	// GitHub refuses an exp more than ten minutes out.
	return c, c.Iat <= now && now < c.Exp && c.Exp-c.Iat <= 600
}

func newTokenHarness(t *testing.T, pemFor func(*rsa.PrivateKey) []byte) (*AppTokenSource, *tokenEndpoint, *fakeClock) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
	endpoint := &tokenEndpoint{t: t, pub: &key.PublicKey, clock: clock, expires: time.Hour}
	srv := httptest.NewServer(endpoint)
	t.Cleanup(srv.Close)

	src, err := NewAppTokenSource(AppTokenOptions{
		Credentials: AppCredentials{AppID: 123, InstallationID: 456, PrivateKeyPEM: pemFor(key)},
		APIURL:      srv.URL,
		Repo:        Repo{Owner: "fuseone-io", Name: "fuseone-agents"},
		HTTP:        srv.Client(),
		Clock:       clock,
	})
	if err != nil {
		t.Fatalf("new token source: %v", err)
	}
	return src, endpoint, clock
}

func pkcs1PEM(k *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
}

func pkcs8PEM(k *rsa.PrivateKey) []byte {
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func TestAppTokenSource_mintsWithAJWTGitHubAccepts_scopedToTheRepository(t *testing.T) {
	t.Parallel()

	for name, enc := range map[string]func(*rsa.PrivateKey) []byte{"pkcs1": pkcs1PEM, "pkcs8": pkcs8PEM} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src, endpoint, _ := newTokenHarness(t, enc)
			tok, err := src.Token(t.Context())
			if err != nil {
				t.Fatalf("token: %v", err)
			}
			if tok != "ghs_minted_1" {
				t.Fatalf("token = %q", tok)
			}
			if strings.Join(endpoint.scoped, ",") != "fuseone-agents" {
				t.Fatalf("token scoped to %v, want only the allowed repository", endpoint.scoped)
			}
		})
	}
}

func TestAppTokenSource_tokenStillFresh_reusedWithoutMinting(t *testing.T) {
	t.Parallel()

	src, endpoint, clock := newTokenHarness(t, pkcs1PEM)
	first, _ := src.Token(t.Context())
	clock.Advance(50 * time.Minute)
	second, err := src.Token(t.Context())
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if first != second || endpoint.mints != 1 {
		t.Fatalf("first %q second %q mints %d, want one reused token", first, second, endpoint.mints)
	}
}

// A token about to expire is replaced before it is used: a merge that starts
// with seconds left on the token would fail half-way through its reads.
func TestAppTokenSource_tokenNearExpiry_mintsANewOne(t *testing.T) {
	t.Parallel()

	src, endpoint, clock := newTokenHarness(t, pkcs1PEM)
	_, _ = src.Token(t.Context())
	clock.Advance(56 * time.Minute)
	tok, err := src.Token(t.Context())
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if tok != "ghs_minted_2" || endpoint.mints != 2 {
		t.Fatalf("token %q after %d mints, want a fresh one", tok, endpoint.mints)
	}
}

func TestNewAppTokenSource_unparseableKey_refused(t *testing.T) {
	t.Parallel()

	_, err := NewAppTokenSource(AppTokenOptions{
		Credentials: AppCredentials{AppID: 1, InstallationID: 2, PrivateKeyPEM: []byte("not a key")},
	})
	if err == nil {
		t.Fatal("an unparseable key was accepted")
	}
}
