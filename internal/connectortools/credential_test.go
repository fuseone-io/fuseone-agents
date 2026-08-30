package connectortools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
The canary is shaped to survive being encoded, because a leak rarely arrives
as the string that was issued.

Percent, quote, slash, backslash and a newline each get rewritten by something
on the way out — JSON escaping, URL encoding, a log formatter folding lines.
A marker made only of letters would prove the value was not copied and nothing
about the value being transformed and copied.
*/
// leaseCanary is the revocation handle. It is not the credential, and it is
// not safe provenance either: it is a handle that acts, and it describes the
// internal shape of the lease. Slice 4 writes a record of every operation, so
// the moment to prove it is not in Issuance is before that record exists.
const leaseCanary = "database/creds/app-x-readonly/LEASE-CANARY-9d1"

const credentialCanary = `CAN%41RY-"6f2b"/x\y` + "\n" + `tail`

// encodings are the forms the same secret takes on its way somewhere. A leak
// test that looks only for the raw bytes passes while the value sits in a log
// line as \u0022 or %22.
func encodings(secret string) []string {
	escaped, _ := json.Marshal(secret)
	return []string{
		secret,
		strings.Trim(string(escaped), `"`),
		url.QueryEscape(secret),
		url.PathEscape(secret),
		strings.ReplaceAll(secret, "\n", ""),
	}
}

// leaks reports every place a credential could reach that is not the SQL
// runtime: an error, a log line, a serialisation, a formatted value. The
// helper is the point — slice 4 widens the same canary to ledger steps,
// content artifacts and tool results without inventing a second definition of
// what a leak looks like.
func leaks(t *testing.T, subject any) bool {
	t.Helper()
	rendered := []string{
		fmt.Sprint(subject), fmt.Sprintf("%v", subject), fmt.Sprintf("%+v", subject),
		fmt.Sprintf("%#v", subject), fmt.Sprintf("%s", subject),
	}
	if encoded, err := json.Marshal(subject); err == nil {
		rendered = append(rendered, string(encoded))
	}
	for _, form := range rendered {
		for _, shape := range encodings(credentialCanary) {
			if shape == "" {
				continue
			}
			if strings.Contains(form, shape) {
				t.Errorf("the credential reached a rendered form as %q: %s", shape, form)
				return true
			}
		}
	}
	return false
}

type vaultIssuer struct {
	calls     int
	username  string
	password  string
	leaseID   string
	ttl       int
	err       error
	revokeErr error
	revoked   string
}

func (v *vaultIssuer) WriteSecret(context.Context, VaultConfig, string, string, map[string]VaultSecretField) (VaultWriteResult, error) {
	return VaultWriteResult{}, nil
}

func (v *vaultIssuer) ReadMetadata(context.Context, VaultConfig, string, string) (VaultMetadata, error) {
	return VaultMetadata{}, nil
}

/*
RevokeLease honours the context, because that is the property under test.

A fake that ignored it would record a revocation the real client could never
have performed — and the guarantee here is precisely that revocation does not
ride the run's cancelled context.
*/
func (v *vaultIssuer) RevokeLease(ctx context.Context, _ VaultConfig, _, leaseID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if v.revokeErr != nil {
		return v.revokeErr
	}
	v.revoked = leaseID
	return nil
}

func (v *vaultIssuer) IssueDatabaseCredential(
	_ context.Context, _ VaultConfig, _, _, _ string,
) (IssuedCredential, error) {
	v.calls++
	if v.err != nil {
		return IssuedCredential{}, v.err
	}
	wire := databaseCredentialWire{LeaseID: v.leaseID, LeaseDuration: v.ttl}
	wire.Data.Username, wire.Data.Password = v.username, v.password
	return wire.issued()
}

func issuer() *vaultIssuer {
	return &vaultIssuer{
		username: "v-token-app-x-1", password: credentialCanary,
		leaseID: leaseCanary, ttl: 300,
	}
}

func runScope() domain.Scope { return area("acme", "platform") }

func configured(instances ...Instance) *CredentialResolver {
	return NewCredentialResolver(
		staticConfig(instances), tokenFor(instances), issuer())
}

type staticConfig []Instance

func (s staticConfig) ConfiguredInstances(context.Context) ([]ConfiguredInstance, error) {
	out := make([]ConfiguredInstance, 0, len(s))
	for _, instance := range s {
		out = append(out, ConfiguredInstance{Instance: instance, HasToken: instance.HasToken})
	}
	return out, nil
}

type tokenFor []Instance

func (t tokenFor) RevealVaultToken(_ context.Context, instance ConfiguredInstance) (string, error) {
	for _, known := range t {
		if known.Name == instance.Name && known.HasToken {
			return "vault-token", nil
		}
	}
	return "", errors.New("no token")
}

// cancelledTokens stands for a store interrupted mid-read.
type cancelledTokens struct{}

func (cancelledTokens) RevealVaultToken(context.Context, ConfiguredInstance) (string, error) {
	return "", context.Canceled
}

func ready() []Instance {
	vault := vaultInstance("prod", company("acme"))
	vault.Token, vault.HasToken = "", true
	return []Instance{vault, sqlInstance(runScope(), vaultRole("prod"))}
}

/*
An approved operation gets a credential, and the caller gets facts about it.

The resolver is told a SQL instance name and a run scope, and nothing else.
Mount and role come from the stored binding: a caller that could pass them
would be able to compose an arbitrary Vault path with a SQL destination, which
is the composition #52 exists to prevent.
*/
func TestCredentialResolver_issuesFromTheStoredBinding(t *testing.T) {
	t.Parallel()

	resolver := configured(ready()...)
	authority, err := resolver.Resolve(context.Background(), "app-x", runScope())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if authority.Credential().Password() != credentialCanary {
		t.Fatalf("the SQL runtime cannot read the credential it was given")
	}
	issued := authority.Issuance()
	if issued.VaultInstance != "prod" || issued.Role != "app-x-readonly" || issued.LeaseTTLSeconds != 300 {
		t.Fatalf("issuance = %+v, want the safe binding and duration", issued)
	}
}

/*
The credential renders redacted in every form Go offers.

String alone is not enough: %#v reaches the struct, and a struct of strings
serialises. Each of these is a way a credential reaches a log or a response
without anybody deciding to put it there.
*/
func TestCredential_isRedactedInEveryRenderedForm(t *testing.T) {
	t.Parallel()

	resolver := configured(ready()...)
	authority, err := resolver.Resolve(context.Background(), "app-x", runScope())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	credential := authority.Credential()
	leaks(t, authority)
	leaks(t, &authority)
	leaks(t, credential)
	leaks(t, authority.Issuance())

	// Safe provenance says which binding answered and for how long, and cannot
	// be used to act. A lease id in here would ride into whatever records the
	// operation.
	encodedIssuance, err := json.Marshal(authority.Issuance())
	if err != nil {
		t.Fatalf("marshal issuance: %v", err)
	}
	if strings.Contains(string(encodedIssuance), "LEASE-CANARY") {
		t.Errorf("safe provenance carries the revocation handle: %s", encodedIssuance)
	}
	leaks(t, struct{ A Authority }{authority})

	// The unexported fields are what make a credential unserialisable; the
	// marshaller is what makes the result legible instead of an empty object.
	// Asserted so removing it is a visible decision rather than a tidy-up.
	encoded, err := json.Marshal(credential)
	if err != nil || string(encoded) != `"[redacted credential]"` {
		t.Fatalf("json = %s, %v; want an explicit redaction", encoded, err)
	}
}

/*
Every part of an incomplete answer refuses on its own.

A credential with no lease id cannot be revoked and one with no TTL cannot be
bounded, so each is a refusal by itself. Driving them together would let one
check cover for the other's removal, which is how a rule stops being load
bearing without anything failing.
*/
func TestCredentialResolver_refusesEachMissingPartOfTheAnswer(t *testing.T) {
	t.Parallel()

	for name, spy := range map[string]*vaultIssuer{
		"no username": {password: credentialCanary, leaseID: "l", ttl: 300},
		"no password": {username: "u", leaseID: "l", ttl: 300},
		"no lease id": {username: "u", password: credentialCanary, ttl: 300},
		"no ttl":      {username: "u", password: credentialCanary, leaseID: "l"},
	} {
		resolver := NewCredentialResolver(staticConfig(ready()), tokenFor(ready()), spy)
		_, err := resolver.Resolve(context.Background(), "app-x", runScope())
		if !errors.Is(err, ErrCredentialIncomplete) {
			t.Errorf("%s: err = %v, want an incomplete credential refused", name, err)
		}
	}
}

func TestCredentialResolver_failuresCarryNothingFromVault(t *testing.T) {
	t.Parallel()

	for name, spy := range map[string]*vaultIssuer{
		"refused":   {err: fmt.Errorf("vault: status 403: %s", credentialCanary)},
		"malformed": {username: "u", password: credentialCanary, leaseID: "", ttl: 0},
		"cancelled": {err: context.Canceled},
	} {
		resolver := NewCredentialResolver(staticConfig(ready()), tokenFor(ready()), spy)
		_, err := resolver.Resolve(context.Background(), "app-x", runScope())
		if err == nil {
			t.Errorf("%s: Resolve returned no error", name)
			continue
		}
		if strings.Contains(err.Error(), credentialCanary) {
			t.Errorf("%s: the error carries vault content: %v", name, err)
		}
	}
}

// Cancellation stays itself, so a caller can tell a run that was stopped from a
// configuration that is wrong.
func TestCredentialResolver_cancellationIsNotFlattened(t *testing.T) {
	t.Parallel()

	resolver := NewCredentialResolver(
		staticConfig(ready()), tokenFor(ready()), &vaultIssuer{err: context.Canceled})
	_, err := resolver.Resolve(context.Background(), "app-x", runScope())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled preserved", err)
	}
}

/*
Two instances answering one name refuse before Vault is contacted.

The configuration refuses ambiguity when it is written; the resolver must not
reintroduce choice-by-order when it reads. The spy counts issuance, so a
resolver that picked one and carried on fails here rather than in production.
*/
func TestCredentialResolver_refusesAmbiguityBeforeContactingVault(t *testing.T) {
	t.Parallel()

	for name, instances := range map[string][]Instance{
		"two vaults": append(ready(), vaultInstance("prod", runScope())),
		"two sql":    append(ready(), sqlInstance(company("acme"), vaultRole("prod"))),
	} {
		spy := issuer()
		resolver := NewCredentialResolver(staticConfig(instances), tokenFor(instances), spy)
		if _, err := resolver.Resolve(context.Background(), "app-x", runScope()); err == nil {
			t.Errorf("%s: ambiguity was resolved instead of refused", name)
		}
		if spy.calls != 0 {
			t.Errorf("%s: vault was contacted %d times before the refusal", name, spy.calls)
		}
	}
}

func TestCredentialResolver_refusesWhatConfigurationNoLongerSupports(t *testing.T) {
	t.Parallel()

	disabled := ready()
	disabled[0].Enabled = false
	outOfScope := []Instance{
		vaultInstance("prod", area("acme", "payments")),
		sqlInstance(runScope(), vaultRole("prod")),
	}
	sqlDisabled := ready()
	sqlDisabled[1].Enabled = false

	for name, instances := range map[string][]Instance{
		"vault disabled":  disabled,
		"vault elsewhere": outOfScope,
		"sql disabled":    sqlDisabled,
		"nothing stored":  {},
	} {
		spy := issuer()
		resolver := NewCredentialResolver(staticConfig(instances), tokenFor(instances), spy)
		if _, err := resolver.Resolve(context.Background(), "app-x", runScope()); err == nil {
			t.Errorf("%s: resolved anyway", name)
		}
		if spy.calls != 0 {
			t.Errorf("%s: vault was contacted %d times", name, spy.calls)
		}
	}
}

// A run outside the SQL instance's scope gets no credential, and Vault is not
// asked. This is the cross-talk property of #15, at the moment authority is
// handed out.
func TestCredentialResolver_refusesARunOutsideTheInstanceScope(t *testing.T) {
	t.Parallel()

	spy := issuer()
	resolver := NewCredentialResolver(staticConfig(ready()), tokenFor(ready()), spy)
	_, err := resolver.Resolve(context.Background(), "app-x", area("acme", "payments"))
	if err == nil {
		t.Fatal("a run outside the instance scope was given a credential")
	}
	if spy.calls != 0 {
		t.Fatalf("vault was contacted %d times for an out-of-scope run", spy.calls)
	}
}

/*
The path is built from validated parts, never from what a binding happens to
contain.

Mount and role reach `/v1/{mount}/creds/{role}`. A mount that climbs out with
`..`, or a role carrying a slash, would address a different Vault endpoint than
the one an administrator wrote down — and the binding is exactly the field an
operator edits by hand.
*/
func TestVaultCredentialPath_refusesAnythingThatCouldAddressElsewhere(t *testing.T) {
	t.Parallel()

	for _, bad := range [][2]string{
		{"database/../sys", "app-x"},
		{"/database", "app-x"},
		{"database", "app-x/../../sys/policy"},
		{"database", "app x"},
		{"", "app-x"},
		{"database", ""},
		{"database\n", "app-x"},
	} {
		if _, err := vaultCredentialPath(bad[0], bad[1]); err == nil {
			t.Errorf("mount %q role %q was accepted", bad[0], bad[1])
		}
	}
	got, err := vaultCredentialPath("database", "app-x-readonly")
	if err != nil || got != "database/creds/app-x-readonly" {
		t.Fatalf("path = %q, %v", got, err)
	}
	nested, err := vaultCredentialPath("db/prod", "app-x")
	if err != nil || nested != "db/prod/creds/app-x" {
		t.Fatalf("nested path = %q, %v", nested, err)
	}
}

/*
The canary crosses a real HTTP response and the real decoder.

Every other test here hands the resolver a fake issuer, so none of them
exercises the one place the bytes arrive from outside: the wire struct, the
JSON decoder and the conversion. This drives an httptest server that answers
with the canary and asserts nothing printable survives the client.
*/
func TestIssueDatabaseCredential_returnsNothingPrintableFromTheWire(t *testing.T) {
	t.Parallel()

	body := map[string]any{
		"lease_id":       "database/creds/app-x/abc",
		"lease_duration": 300,
		"data":           map[string]any{"username": "v-app-x", "password": credentialCanary},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/database/creds/app-x-readonly" {
			t.Errorf("path = %q, want the bound mount and role", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()

	client := NewHTTPVaultClient(server.Client())
	issued, err := client.IssueDatabaseCredential(context.Background(),
		VaultConfig{Address: server.URL}, "vault-token", "database", "app-x-readonly")
	if err != nil {
		t.Fatalf("IssueDatabaseCredential: %v", err)
	}
	leaks(t, issued)
	leaks(t, &issued)
	leaks(t, struct{ I IssuedCredential }{issued})
	if issued.credential.Password() != credentialCanary {
		t.Fatal("the credential did not survive the wire")
	}
}

// A non-2xx answer says the status and nothing Vault put in the body.
func TestIssueDatabaseCredential_carriesNothingFromAnErrorBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["` + credentialCanary + `"]}`))
	}))
	defer server.Close()

	_, err := NewHTTPVaultClient(server.Client()).IssueDatabaseCredential(
		context.Background(), VaultConfig{Address: server.URL},
		"vault-token", "database", "app-x-readonly")
	if err == nil {
		t.Fatal("a 403 was accepted")
	}
	leaks(t, err.Error())
}

// A token read that was cancelled is a cancelled run, not a configuration
// that cannot answer. Flattening it would send an operator to the settings.
func TestCredentialResolver_cancelledTokenReadStaysCancelled(t *testing.T) {
	t.Parallel()

	spy := issuer()
	resolver := NewCredentialResolver(staticConfig(ready()), cancelledTokens{}, spy)
	_, err := resolver.Resolve(context.Background(), "app-x", runScope())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled preserved", err)
	}
	if spy.calls != 0 {
		t.Errorf("vault was contacted %d times after a cancelled token read", spy.calls)
	}
}

// Giving the lease back goes through the same vault the credential came from,
// with the configuration and token the caller never sees.
func TestAuthority_revokeReturnsTheLeaseItWasGiven(t *testing.T) {
	t.Parallel()

	spy := issuer()
	resolver := NewCredentialResolver(staticConfig(ready()), tokenFor(ready()), spy)
	authority, err := resolver.Resolve(context.Background(), "app-x", runScope())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := authority.Revoke(context.Background()); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if spy.revoked != leaseCanary {
		t.Fatalf("revoked = %q, want the lease that was issued", spy.revoked)
	}
}
