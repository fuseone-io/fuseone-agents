package connectortools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

const credentialCanary = "CREDENTIAL-CANARY-6f2b"

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
		if strings.Contains(form, credentialCanary) {
			t.Errorf("the credential reached a rendered form: %s", form)
			return true
		}
	}
	return false
}

type vaultIssuer struct {
	calls    int
	username string
	password string
	leaseID  string
	ttl      int
	err      error
}

func (v *vaultIssuer) WriteSecret(context.Context, VaultConfig, string, string, map[string]VaultSecretField) (VaultWriteResult, error) {
	return VaultWriteResult{}, nil
}

func (v *vaultIssuer) ReadMetadata(context.Context, VaultConfig, string, string) (VaultMetadata, error) {
	return VaultMetadata{}, nil
}

func (v *vaultIssuer) RevokeLease(context.Context, VaultConfig, string, string) error { return nil }

func (v *vaultIssuer) IssueDatabaseCredential(
	_ context.Context, _ VaultConfig, _, _, _ string,
) (VaultDatabaseCredential, error) {
	v.calls++
	if v.err != nil {
		return VaultDatabaseCredential{}, v.err
	}
	return VaultDatabaseCredential{
		Username: v.username, Password: v.password,
		LeaseID: v.leaseID, LeaseTTLSeconds: v.ttl,
	}, nil
}

func issuer() *vaultIssuer {
	return &vaultIssuer{
		username: "v-token-app-x-1", password: credentialCanary,
		leaseID: "database/creds/app-x-readonly/abc", ttl: 300,
	}
}

func runScope() domain.Scope { return area("acme", "platform") }

func configured(instances ...Instance) *CredentialResolver {
	return NewCredentialResolver(
		staticConfig(instances), tokenFor(instances), issuer())
}

type staticConfig []Instance

func (s staticConfig) Instances(context.Context) ([]Instance, error) { return []Instance(s), nil }

type tokenFor []Instance

func (t tokenFor) VaultToken(_ context.Context, name string, scope domain.Scope) (string, error) {
	for _, instance := range t {
		if instance.Name == name && instance.Scope.Contains(scope) {
			return "vault-token", nil
		}
	}
	return "", errors.New("no token")
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
	credential, issued, err := resolver.Resolve(context.Background(), "app-x", runScope())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if credential.Password() != credentialCanary {
		t.Fatalf("the SQL runtime cannot read the credential it was given")
	}
	if issued.VaultInstance != "prod" || issued.Role != "app-x-readonly" || issued.LeaseTTLSeconds != 300 {
		t.Fatalf("issuance = %+v, want the safe binding and duration", issued)
	}
	if issued.LeaseID == "" {
		t.Error("the lease cannot be revoked without its id")
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
	credential, issued, err := resolver.Resolve(context.Background(), "app-x", runScope())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	leaks(t, credential)
	leaks(t, &credential)
	leaks(t, issued)
	leaks(t, struct{ C Credential }{credential})

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
		_, _, err := resolver.Resolve(context.Background(), "app-x", runScope())
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
		_, _, err := resolver.Resolve(context.Background(), "app-x", runScope())
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
	_, _, err := resolver.Resolve(context.Background(), "app-x", runScope())
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
		if _, _, err := resolver.Resolve(context.Background(), "app-x", runScope()); err == nil {
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
		if _, _, err := resolver.Resolve(context.Background(), "app-x", runScope()); err == nil {
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
	_, _, err := resolver.Resolve(context.Background(), "app-x", area("acme", "payments"))
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
