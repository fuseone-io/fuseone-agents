package admin_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

// An installation with no identity provider has exactly one person in it: the
// one who claimed the setup token. Everything here is about the moment a
// second person becomes possible — and about the fact that being able to sign
// in is not the same as being allowed to do anything.

// newIdentity returns the store and the pool it writes through. The pool comes
// back because openPool empties the trail as it opens — asking for a second
// one mid-test deletes what the test just recorded.
func newIdentity(t *testing.T) (*admin.Identity, *pgxpool.Pool) {
	t.Helper()

	pool := openPool(t)
	if _, err := pool.Exec(context.Background(),
		`delete from settings where kind = 'identity_provider'`); err != nil {
		t.Fatalf("clean: %v", err)
	}

	key := make([]byte, 32)
	v, err := vault.New(key, "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	return admin.NewIdentity(pool, settings.NewStore(pool, v)), pool
}

// lastTrailDetail is what the trail recorded for the most recent change to a
// target, as raw JSON.
func lastTrailDetail(t *testing.T, pool *pgxpool.Pool, target string) (action, detail string) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `
		select action, coalesce(detail::text, '')
		from admin_events where target = $1 order by event_id desc limit 1`, target,
	).Scan(&action, &detail)
	if err != nil {
		t.Fatalf("read trail: %v", err)
	}
	return action, detail
}

func TestPutIdentityProvider_withoutAnIssuer_isRefused(t *testing.T) {
	// Discovery is what makes an assertion verifiable. A provider with no
	// issuer cannot be discovered, so it could only ever fail at sign-in —
	// where the operator is not looking.
	store, _ := newIdentity(t)
	err := store.PutIdentityProvider(context.Background(), "ana", domain.Scope{},
		domain.IdentityProvider{ID: "keycloak", Display: "Keycloak"}, "")
	if err == nil {
		t.Fatal("want a refusal")
	}
}

func TestPutIdentityProvider_storesTheMappingsAndHidesTheSecret(t *testing.T) {
	store, _ := newIdentity(t)
	ctx := context.Background()

	err := store.PutIdentityProvider(ctx, "ana", domain.Scope{}, domain.IdentityProvider{
		ID: "keycloak", Display: "Keycloak", Issuer: "https://id.example/realms/acme",
		ClientID: "fuseone", GroupsClaim: "groups", Enabled: true,
		Mappings: []domain.GroupMapping{
			{Group: "suporte", Company: "acme", Area: "cx", Role: "author"},
		},
	}, "s3cr3t")
	if err != nil {
		t.Fatalf("PutIdentityProvider: %v", err)
	}

	got, err := store.IdentityProviders(ctx)
	if err != nil {
		t.Fatalf("IdentityProviders: %v", err)
	}
	if len(got) != 1 || got[0].ID != "keycloak" {
		t.Fatalf("providers = %+v", got)
	}
	if len(got[0].Mappings) != 1 || got[0].Mappings[0].Role != "author" {
		t.Errorf("mappings = %+v, want the one that was configured", got[0].Mappings)
	}
	// Listing configuration is routine; reading a credential is not, and the
	// listing must never be the place it leaks.
	if got[0].ClientSecret != "" {
		t.Error("the listing carried the client secret")
	}
	if !got[0].HasSecret {
		t.Error("the listing does not say a secret is stored")
	}

	secret, err := store.IdentitySecret(ctx, "keycloak")
	if err != nil {
		t.Fatalf("IdentitySecret: %v", err)
	}
	if secret != "s3cr3t" {
		t.Errorf("secret = %q", secret)
	}
}

func TestPutIdentityProvider_withNoSecret_keepsTheStoredOne(t *testing.T) {
	store, _ := newIdentity(t)
	ctx := context.Background()
	provider := domain.IdentityProvider{
		ID: "keycloak", Display: "Keycloak", Issuer: "https://id.example/realms/acme",
		ClientID: "fuseone", Enabled: true,
	}
	if err := store.PutIdentityProvider(ctx, "ana", domain.Scope{}, provider, "s3cr3t"); err != nil {
		t.Fatalf("first: %v", err)
	}

	// Changing a group mapping must not demand re-entering the credential:
	// somebody who cannot produce it would leave the mapping wrong instead.
	provider.Mappings = []domain.GroupMapping{{Group: "cx", Company: "acme", Area: "cx", Role: "approver"}}
	if err := store.PutIdentityProvider(ctx, "ana", domain.Scope{}, provider, ""); err != nil {
		t.Fatalf("second: %v", err)
	}

	secret, err := store.IdentitySecret(ctx, "keycloak")
	if err != nil {
		t.Fatalf("IdentitySecret: %v", err)
	}
	if secret != "s3cr3t" {
		t.Errorf("secret = %q, want the stored one kept", secret)
	}
}

func TestPutIdentityProvider_isRecordedInTheTrail(t *testing.T) {
	store, pool := newIdentity(t)
	ctx := context.Background()
	if err := store.PutIdentityProvider(ctx, "ana", domain.Scope{}, domain.IdentityProvider{
		ID: "keycloak", Display: "Keycloak", Issuer: "https://id.example/realms/acme",
		ClientID: "fuseone", Enabled: true,
	}, "s3cr3t"); err != nil {
		t.Fatalf("PutIdentityProvider: %v", err)
	}

	// Who may sign in and what they get is the most consequential thing an
	// operator can change. The trail records it, and never the secret.
	action, detail := lastTrailDetail(t, pool, "keycloak")
	if action != "identity.provider.configured" {
		t.Errorf("action = %q", action)
	}
	if contains(detail, "s3cr3t") {
		t.Error("the trail carried the client secret")
	}
}
