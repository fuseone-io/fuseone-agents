package githubapp_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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

// Eight calls landing in the refresh window while GitHub is failing try the
// mint once between them: the rest keep the still-valid token without asking
// GitHub again, so an outage is not multiplied by the worker's concurrency.
func TestSource_concurrentCallsAfterAFailedRefresh_mintOnceAndKeepTheToken(t *testing.T) {
	t.Parallel()
	fake := githubapptest.New(t, appID, installationID)
	clock := newClock()
	fake.UseClock(clock.Now)
	src := source(t, appFor(fake), clock)

	first, _ := src.Token(t.Context())
	clock.advance(57 * time.Minute)
	fake.Refuse(http.StatusBadGateway)
	var wg sync.WaitGroup
	got := make([]string, 8)
	for i := range got {
		wg.Go(func() { got[i], _ = src.Token(t.Context()) })
	}
	wg.Wait()

	if attempts := len(fake.JWTs()); attempts != 2 {
		t.Fatalf("GitHub was asked %d times, want 2: the first mint and one failed refresh", attempts)
	}
	for i, tok := range got {
		if tok != first {
			t.Fatalf("call %d got %q, want the still-valid token %q", i, tok, first)
		}
	}
}

// With no token in hand, a failure is remembered for the backoff and returned
// without a new mint; once the backoff passes, the next call asks again.
func TestSource_aFailedMint_isRememberedForTheBackoffThenRetried(t *testing.T) {
	t.Parallel()
	fake := githubapptest.New(t, appID, installationID)
	clock := newClock()
	fake.UseClock(clock.Now)
	fake.Refuse(http.StatusUnauthorized)
	src := source(t, appFor(fake), clock)

	_, first := src.Token(t.Context())
	_, again := src.Token(t.Context())
	asked := len(fake.JWTs())
	clock.advance(githubapp.RetryAfter)
	fake.Refuse(0)
	tok, err := src.Token(t.Context())

	if !errors.Is(first, githubapp.ErrAppRefused) || !errors.Is(again, githubapp.ErrAppRefused) {
		t.Fatalf("errors %v / %v, want the refusal both times", first, again)
	}
	if asked != 1 {
		t.Fatalf("GitHub was asked %d times inside the backoff, want 1", asked)
	}
	if err != nil || !fake.Valid(tok) {
		t.Fatalf("after the backoff Token = %q, %v; want a fresh token", tok, err)
	}
}

// A caller that gave up is not GitHub failing: its cancelled context must not
// be remembered and handed to the next caller for the whole backoff.
func TestSource_aCancelledCaller_isNotRememberedAsAFailedMint(t *testing.T) {
	t.Parallel()
	fake := githubapptest.New(t, appID, installationID)
	clock := newClock()
	fake.UseClock(clock.Now)
	src := source(t, appFor(fake), clock)

	gone, cancel := context.WithCancel(t.Context())
	cancel()
	_, _ = src.Token(gone)
	tok, err := src.Token(t.Context())

	if err != nil || !fake.Valid(tok) {
		t.Fatalf("Token after a cancelled caller = %q, %v; want a fresh token", tok, err)
	}
}

// Every request a GitHub App makes carries a secret — the App JWT to the API,
// the installation token to the MCP server — so plain HTTP is refused unless
// the host is this machine, where nothing crosses a wire.
func TestRequireTLS_plainHTTPOnlyToLoopback(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"https://ghe.internal.example/api/v3": true,
		"http://127.0.0.1:8080/api/v3":        true,
		"http://127.9.9.9/":                   true,
		"http://[::1]:9000/":                  true,
		"http://localhost:3000/":              true,
		"http://ghe.internal.example/api/v3":  false,
		"http://10.0.0.5/":                    false,
		"http://127.0.0.1.attacker.example/":  false,
		"http://localhost.attacker.example/":  false,
		"ftp://ghe.internal.example/":         false,
	}
	for raw, ok := range cases {
		err := githubapp.RequireTLS(raw)
		if (err == nil) != ok {
			t.Errorf("RequireTLS(%q) = %v, want allowed=%v", raw, err, ok)
		}
		if err != nil && !errors.Is(err, githubapp.ErrPlainHTTP) {
			t.Errorf("RequireTLS(%q) = %v, want ErrPlainHTTP", raw, err)
		}
	}
}

// Credential headers are dropped in any letter case, whatever stored them;
// configuration headers pass untouched.
func TestConfigHeaders_dropsCredentialHeadersInAnyCase(t *testing.T) {
	t.Parallel()
	given := map[string]string{
		"authorization":       "Bearer personal",
		"PROXY-AUTHORIZATION": "Basic cHJveHk=",
		"X-MCP-Toolsets":      "issues,projects",
	}

	got := githubapp.ConfigHeaders(given)

	if len(got) != 1 || got["X-MCP-Toolsets"] != "issues,projects" {
		t.Fatalf("ConfigHeaders = %v, want only X-MCP-Toolsets", got)
	}
	if !errors.Is(githubapp.CheckConfigHeaders(given), githubapp.ErrCredentialHeader) {
		t.Fatal("CheckConfigHeaders accepted a credential header")
	}
}

// countingTransport records requests and answers none of them.
type countingTransport struct{ requests atomic.Int32 }

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.requests.Add(1)
	return nil, errors.New("no network in this test")
}

// A stored base URL that is plain HTTP to another host never receives the
// JWT: the mint refuses before a request is built, whatever admin accepted.
func TestSource_plainHTTPAPIBase_refusesBeforeTheJWTLeaves(t *testing.T) {
	t.Parallel()
	fake := githubapptest.New(t, appID, installationID)
	app := appFor(fake)
	app.APIBaseURL = "http://ghe.internal.example/api/v3"
	wire := &countingTransport{}
	src, err := githubapp.NewSource(app, &http.Client{Transport: wire}, newClock())
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}

	_, err = src.Token(t.Context())

	if !errors.Is(err, githubapp.ErrPlainHTTP) || wire.requests.Load() != 0 {
		t.Fatalf("Token err = %v after %d requests; want ErrPlainHTTP and none sent",
			err, wire.requests.Load())
	}
}

// A redirect from the API base is never followed: Go resends Authorization to
// the same host on another scheme, so an https base that answers 302 to
// http:// would hand the App JWT over in clear. The mint fails instead.
func TestSource_aRedirectFromTheAPIBase_isNotFollowed(t *testing.T) {
	t.Parallel()
	fake := githubapptest.New(t, appID, installationID)
	var plain atomic.Int32
	cleartext := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		plain.Add(1)
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(cleartext.Close)
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, cleartext.URL+r.URL.Path, http.StatusFound)
	}))
	t.Cleanup(secure.Close)
	app := appFor(fake)
	app.APIBaseURL = secure.URL
	client := secure.Client()
	src, err := githubapp.NewSource(app, client, newClock())
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}

	_, err = src.Token(t.Context())

	if !errors.Is(err, githubapp.ErrMintFailed) || plain.Load() != 0 {
		t.Fatalf("Token err = %v, cleartext listener hit %d times; want a mint failure and no hop",
			err, plain.Load())
	}
	if client.CheckRedirect != nil {
		t.Fatal("the caller's client was changed; the mint must use its own copy")
	}
	assertNoSecret(t, err.Error(), app.PrivateKey, nil)
}

// slowGitHub advances the clock while a mint is in flight, the way a slow
// GitHub does: the call returns later than it started.
type slowGitHub struct {
	clock *stepClock
	delay atomic.Int64
	next  http.RoundTripper
}

func (s *slowGitHub) RoundTrip(r *http.Request) (*http.Response, error) {
	s.clock.advance(time.Duration(s.delay.Load()))
	return s.next.RoundTrip(r)
}

func slowSource(t *testing.T, fake *githubapptest.Fake, clock *stepClock) (*githubapp.Source, *slowGitHub) {
	t.Helper()
	slow := &slowGitHub{clock: clock, next: http.DefaultTransport}
	src, err := githubapp.NewSource(appFor(fake), &http.Client{Transport: slow}, clock)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	return src, slow
}

// The backoff starts when the failed mint returned, not when it began: a mint
// that takes longer than RetryAfter must not have used the backoff up already.
func TestSource_aSlowFailedMint_startsTheBackoffWhenItReturns(t *testing.T) {
	t.Parallel()
	fake := githubapptest.New(t, appID, installationID)
	clock := newClock()
	fake.UseClock(clock.Now)
	src, slow := slowSource(t, fake, clock)

	first, _ := src.Token(t.Context())
	clock.advance(57 * time.Minute)
	fake.Refuse(http.StatusBadGateway)
	slow.delay.Store(int64(20 * time.Second))
	_, _ = src.Token(t.Context())
	got, err := src.Token(t.Context())

	if attempts := len(fake.JWTs()); attempts != 2 {
		t.Fatalf("GitHub was asked %d times, want 2: the first mint and one slow failed refresh", attempts)
	}
	if err != nil || got != first {
		t.Fatalf("Token = %q, %v; want the still-valid token", got, err)
	}
}

// Whether the old token is still worth sending is judged when the failed mint
// returns: with 45s left at the start and 25s left at the end, it is not.
func TestSource_aSlowFailedMint_judgesTheOldTokenWhenItReturns(t *testing.T) {
	t.Parallel()
	fake := githubapptest.New(t, appID, installationID)
	clock := newClock()
	fake.UseClock(clock.Now)
	src, slow := slowSource(t, fake, clock)

	_, _ = src.Token(t.Context())
	clock.advance(59*time.Minute + 15*time.Second)
	fake.Refuse(http.StatusBadGateway)
	slow.delay.Store(int64(20 * time.Second))
	got, err := src.Token(t.Context())

	if !errors.Is(err, githubapp.ErrMintFailed) {
		t.Fatalf("Token = %q, %v; want the failure, the old token having under 30s left", got, err)
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
