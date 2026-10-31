package githubapp_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/githubapp"
	"github.com/fuseone/agents/internal/githubapp/githubapptest"
)

const (
	appID          = 5550564
	installationID = 77
)

// stepClock is a clock a test moves by hand.
type stepClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *stepClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

func newClock() *stepClock {
	return &stepClock{at: time.Now().UTC().Truncate(time.Second)}
}

func appFor(fake *githubapptest.Fake) domain.MCPGitHubApp {
	return domain.MCPGitHubApp{
		AppID: fake.AppID, InstallationID: fake.InstallationID,
		PrivateKey: fake.PKCS1(), APIBaseURL: fake.Base(),
	}
}

func source(t *testing.T, app domain.MCPGitHubApp, clock githubapp.Clock) *githubapp.Source {
	t.Helper()
	src, err := githubapp.NewSource(app, http.DefaultClient, clock)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	return src
}

// GitHub hands out PKCS#1; `openssl pkcs8 -topk8` turns it into PKCS#8. An
// operator pastes whichever they have, and both are the same key.
func TestParsePrivateKey_pkcs1AndPkcs8_bothRead(t *testing.T) {
	t.Parallel()
	fake := githubapptest.New(t, appID, installationID)

	for name, text := range map[string]string{"pkcs1": fake.PKCS1(), "pkcs8": fake.PKCS8(t)} {
		key, err := githubapp.ParsePrivateKey(text)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !key.Equal(fake.Key) {
			t.Fatalf("%s: parsed a different key", name)
		}
	}
}

// A wrong paste is refused with a reason that names the problem and never
// echoes what was pasted.
func TestParsePrivateKey_notAnRSAKey_refusedWithoutEchoingIt(t *testing.T) {
	t.Parallel()
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(ec)
	if err != nil {
		t.Fatal(err)
	}
	ecPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	garbled := "-----BEGIN RSA PRIVATE KEY-----\nTUlJc2VjcmV0\n-----END RSA PRIVATE KEY-----\n"

	cases := []struct {
		name, text string
		want       error
	}{
		{"not pem", "ghp_a-token-pasted-by-mistake", githubapp.ErrKeyNotPEM},
		{"ec key", ecPEM, githubapp.ErrKeyNotRSA},
		{"garbled", garbled, githubapp.ErrKeyUnreadable},
	}
	for _, tc := range cases {
		_, err := githubapp.ParsePrivateKey(tc.text)
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
		if strings.Contains(err.Error(), "ghp_a-token") || strings.Contains(err.Error(), "TUlJ") {
			t.Fatalf("%s: the refusal echoed the paste: %v", tc.name, err)
		}
	}
}

// The token is minted with a JWT GitHub accepts: signed RS256 by the App's
// key, issued by the App, backdated for clock skew and living no more than
// ten minutes. The fake verifies the signature with the public key.
func TestSource_mintsWithAJWTSignedByTheAppKey(t *testing.T) {
	t.Parallel()
	fake := githubapptest.New(t, appID, installationID)
	clock := newClock()
	fake.UseClock(clock.Now)
	app := appFor(fake)
	app.Repositories = []string{"fuseone-agents"}

	token, err := source(t, app, clock).Token(t.Context())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	claims := fake.LastClaims()
	now := clock.Now().Unix()
	switch {
	case token != "ghs_fake_1":
		t.Fatalf("token = %q, want the minted installation token", token)
	case claims.Issuer != "5550564":
		t.Fatalf("iss = %q, want the App id", claims.Issuer)
	case claims.IssuedAt != now-60:
		t.Fatalf("iat = %d, want 60s before now (%d) for clock skew", claims.IssuedAt, now)
	case claims.ExpiresAt-now > 600 || claims.ExpiresAt <= now:
		t.Fatalf("exp = %d, want within ten minutes of now (%d)", claims.ExpiresAt, now)
	case !slices.Equal(fake.LastRepositories(), []string{"fuseone-agents"}):
		t.Fatalf("repositories = %v, want the configured restriction", fake.LastRepositories())
	}
}

// GitHub Enterprise Server roots its API under /api/v3 on its own host.
func TestSource_enterpriseAPIBase_isWhereTheTokenIsMinted(t *testing.T) {
	t.Parallel()
	fake := githubapptest.New(t, appID, installationID)
	fake.Prefix = "/api/v3"

	if _, err := source(t, appFor(fake), newClock()).Token(t.Context()); err != nil {
		t.Fatalf("Token against %s: %v", fake.Base(), err)
	}
}

// One token serves every call until a few minutes before it expires; then the
// next call gets a fresh one, minted before the old one could be refused.
func TestSource_refreshesBeforeExpiry_andNotBefore(t *testing.T) {
	t.Parallel()
	fake := githubapptest.New(t, appID, installationID)
	clock := newClock()
	fake.UseClock(clock.Now)
	src := source(t, appFor(fake), clock)

	first, _ := src.Token(t.Context())
	clock.advance(50 * time.Minute)
	same, _ := src.Token(t.Context())
	clock.advance(6 * time.Minute) // four minutes left on the first token
	fresh, err := src.Token(t.Context())

	if err != nil || same != first || fresh == first || fake.Minted() != 2 {
		t.Fatalf("tokens %q %q %q (err %v), minted %d; want reuse at 50m and a refresh at 56m",
			first, same, fresh, err, fake.Minted())
	}
	if !fake.Valid(fresh) {
		t.Fatal("the refreshed token is not one GitHub would accept")
	}
}

// A refresh that fails while the token in hand is still good keeps using it:
// the next call tries again, and no call fails for a blip at GitHub.
func TestSource_aFailedRefresh_keepsAStillValidToken(t *testing.T) {
	t.Parallel()
	fake := githubapptest.New(t, appID, installationID)
	clock := newClock()
	fake.UseClock(clock.Now)
	src := source(t, appFor(fake), clock)

	first, _ := src.Token(t.Context())
	clock.advance(57 * time.Minute)
	fake.Refuse(http.StatusBadGateway)
	got, err := src.Token(t.Context())

	if err != nil || got != first {
		t.Fatalf("Token = %q, %v; want the still-valid token kept", got, err)
	}
}

// A revoked key and a removed installation are different fixes, so they get
// different reasons — and neither reason carries the key or the JWT.
func TestSource_revokedKeyOrInstallation_failsWithAReasonFreeOfSecrets(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, githubapp.ErrAppRefused},
		{http.StatusNotFound, githubapp.ErrInstallationNotFound},
		{http.StatusForbidden, githubapp.ErrMintForbidden},
		{http.StatusInternalServerError, githubapp.ErrMintFailed},
	}
	for _, tc := range cases {
		fake := githubapptest.New(t, appID, installationID)
		fake.Refuse(tc.status)
		app := appFor(fake)

		_, err := source(t, app, newClock()).Token(t.Context())

		if !errors.Is(err, tc.want) {
			t.Fatalf("HTTP %d: err = %v, want %v", tc.status, err, tc.want)
		}
		assertNoSecret(t, err.Error(), app.PrivateKey, fake.JWTs())
	}
}

// An expired token with GitHub refusing is a failure, not the old token.
func TestSource_aFailedRefreshAfterExpiry_fails(t *testing.T) {
	t.Parallel()
	fake := githubapptest.New(t, appID, installationID)
	clock := newClock()
	fake.UseClock(clock.Now)
	src := source(t, appFor(fake), clock)

	_, _ = src.Token(t.Context())
	clock.advance(61 * time.Minute)
	fake.Refuse(http.StatusUnauthorized)

	if got, err := src.Token(t.Context()); !errors.Is(err, githubapp.ErrAppRefused) {
		t.Fatalf("Token = %q, %v; want the refusal", got, err)
	}
}

func assertNoSecret(t *testing.T, text, key string, jwts []string) {
	t.Helper()
	for _, line := range strings.Split(key, "\n") {
		if len(line) > 16 && strings.Contains(text, line) {
			t.Fatalf("leaked a line of the private key: %s", text)
		}
	}
	for _, jwt := range jwts {
		if jwt != "" && strings.Contains(text, jwt) {
			t.Fatalf("leaked the App JWT: %s", text)
		}
		if parts := strings.Split(jwt, "."); len(parts) == 3 && strings.Contains(text, parts[2]) {
			t.Fatalf("leaked the JWT signature: %s", text)
		}
	}
}
