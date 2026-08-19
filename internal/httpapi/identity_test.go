package httpapi

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// Who may sign in, and what signing in grants them, is the most consequential
// thing an operator can change here. Two properties carry the weight: the
// credential never comes back out, and saving makes the provider live — a
// configuration that needs a restart to take effect is one nobody trusts.

type fakeIdentity struct {
	stored       []domain.IdentityProvider
	secret       string
	putWith      string
	deleted      string
	err          error
	providersErr error
	secretErr    error
	secretReads  int
}

func (f *fakeIdentity) IdentityProviders(context.Context) ([]domain.IdentityProvider, error) {
	if f.providersErr != nil {
		return nil, f.providersErr
	}
	return f.stored, nil
}

func (f *fakeIdentity) PutIdentityProvider(
	_ context.Context, _ domain.UserID, _ domain.Scope, p domain.IdentityProvider, secret string,
) error {
	if f.err != nil {
		return f.err
	}
	f.putWith = secret
	f.stored = append(f.stored, p)
	return nil
}

func (f *fakeIdentity) DeleteIdentityProvider(
	_ context.Context, _ domain.UserID, _ domain.Scope, id string,
) error {
	f.deleted = id
	return nil
}

func (f *fakeIdentity) IdentitySecret(context.Context, string) (string, error) {
	f.secretReads++
	if f.secretErr != nil {
		return "", f.secretErr
	}
	return f.secret, nil
}

type fakeSignIn struct {
	added   []*auth.OIDCProvider
	removed []string
	err     error
}

func (f *fakeSignIn) Add(_ context.Context, p *auth.OIDCProvider) error {
	if f.err != nil {
		return f.err
	}
	f.added = append(f.added, p)
	return nil
}

func (f *fakeSignIn) Remove(id string) { f.removed = append(f.removed, id) }

func identityServer(store *fakeIdentity, live *fakeSignIn) *Server {
	return NewServer(ledger.NewMemory(), "test").WithIdentity(store, live)
}

func providerInput() openapi.PutIdentityProviderRequestObject {
	return openapi.PutIdentityProviderRequestObject{
		Id: "keycloak",
		Body: &openapi.PutIdentityProviderJSONRequestBody{
			Display: "Keycloak", Issuer: "https://id.example/realms/acme", ClientId: "fuseone",
			Mappings: &[]openapi.GroupMapping{
				{Group: "suporte", Company: "acme", Area: "cx", Role: "author"},
			},
		},
	}
}

func TestPutIdentityProvider_savingMakesItLive(t *testing.T) {
	t.Parallel()

	live := &fakeSignIn{}
	store := &fakeIdentity{secret: "stored-secret"}
	resp, err := identityServer(store, live).PutIdentityProvider(asInstallation(domain.RoleCurator), providerInput())
	if err != nil {
		t.Fatalf("PutIdentityProvider: %v", err)
	}
	if _, ok := resp.(openapi.PutIdentityProvider204Response); !ok {
		t.Fatalf("response = %T", resp)
	}

	// A provider that needed a restart to take effect would look configured
	// and refuse every sign-in until somebody thought to bounce the process.
	if len(live.added) != 1 || live.added[0].ID != "keycloak" {
		t.Fatalf("registry = %+v", live.added)
	}
	// Registered with the credential from the vault, since none was sent:
	// otherwise editing a group mapping would quietly re-register the
	// provider with no secret and break the code exchange.
	if live.added[0].ClientSecret != "stored-secret" {
		t.Errorf("secret = %q, want the stored one", live.added[0].ClientSecret)
	}
	if len(live.added[0].Mappings) != 1 || live.added[0].Mappings[0].Role != "author" {
		t.Errorf("mappings = %+v", live.added[0].Mappings)
	}
}

func TestPutIdentityProvider_thatCannotBeDiscovered_saysSoNow(t *testing.T) {
	t.Parallel()

	live := &fakeSignIn{err: errors.New("dial tcp: connection refused")}
	resp, err := identityServer(&fakeIdentity{}, live).PutIdentityProvider(
		asInstallation(domain.RoleCurator), providerInput())
	if err != nil {
		t.Fatalf("PutIdentityProvider: %v", err)
	}

	// In front of the operator who typed the address, rather than later in
	// front of somebody signing in who cannot fix it.
	bad, ok := resp.(openapi.PutIdentityProvider400ApplicationProblemPlusJSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the failure reported", resp)
	}
	if bad.Detail == nil || *bad.Detail == "" {
		t.Error("the refusal does not say what failed")
	}
}

func TestPutIdentityProvider_disabled_leavesTheRegistry(t *testing.T) {
	t.Parallel()

	live := &fakeSignIn{}
	req := providerInput()
	off := false
	req.Body.Enabled = &off

	if _, err := identityServer(&fakeIdentity{}, live).PutIdentityProvider(asInstallation(domain.RoleCurator), req); err != nil {
		t.Fatalf("PutIdentityProvider: %v", err)
	}
	// Switching a provider off keeps its configuration and stops the sign-ins.
	if len(live.added) != 0 || len(live.removed) != 1 {
		t.Errorf("added %d, removed %v", len(live.added), live.removed)
	}
}

func TestListIdentityProviders_neverCarriesTheClientSecret(t *testing.T) {
	t.Parallel()

	store := &fakeIdentity{stored: []domain.IdentityProvider{{
		ID: "keycloak", Display: "Keycloak", Issuer: "https://id.example",
		ClientID: "fuseone", ClientSecret: "s3cr3t", HasSecret: true, Enabled: true,
	}}}

	resp, err := identityServer(store, &fakeSignIn{}).ListIdentityProviders(
		asInstallation(domain.RoleCurator), openapi.ListIdentityProvidersRequestObject{})
	if err != nil {
		t.Fatalf("ListIdentityProviders: %v", err)
	}
	got := resp.(openapi.ListIdentityProviders200JSONResponse)
	if len(got.Items) != 1 || !got.Items[0].HasSecret {
		t.Fatalf("items = %+v", got.Items)
	}
	// The rendered shape has nowhere to put it, and that is the point: a
	// listing is read by every screen and logged by every proxy.
	if got.Items[0].Issuer != "https://id.example" {
		t.Errorf("issuer = %q", got.Items[0].Issuer)
	}
}

func TestPutIdentityProvider_withoutIdentityWrite_isForbidden(t *testing.T) {
	t.Parallel()

	store := &fakeIdentity{}
	live := &fakeSignIn{}
	// Deciding who may sign in is not something an author or an approver does.
	resp, err := identityServer(store, live).PutIdentityProvider(asInstallation(domain.RoleAuthor), providerInput())
	if err != nil {
		t.Fatalf("PutIdentityProvider: %v", err)
	}
	if _, ok := resp.(openapi.PutIdentityProvider403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want it refused", resp)
	}
	if len(store.stored) != 0 || len(live.added) != 0 {
		t.Error("it was configured anyway")
	}
}

func TestDeleteIdentityProvider_stopsAcceptingSignIns(t *testing.T) {
	t.Parallel()

	store, live := &fakeIdentity{}, &fakeSignIn{}
	if _, err := identityServer(store, live).DeleteIdentityProvider(asInstallation(domain.RoleCurator),
		openapi.DeleteIdentityProviderRequestObject{Id: "keycloak"}); err != nil {
		t.Fatalf("DeleteIdentityProvider: %v", err)
	}

	// Removed from the store and from the live registry: leaving it in the
	// second keeps accepting sign-ins for a configuration nobody can see.
	if store.deleted != "keycloak" || len(live.removed) != 1 {
		t.Errorf("deleted %q, removed %v", store.deleted, live.removed)
	}
}

func TestPutIdentityProvider_adminInTheAdministrationAreaCannotGrantSignInToInstallation(t *testing.T) {
	t.Parallel()

	store, live := &fakeIdentity{}, &fakeSignIn{}
	req := providerInput()
	req.Body.Mappings = &[]openapi.GroupMapping{
		{Group: "admins", Company: "*", Area: "", Role: "admin"},
	}

	resp, err := identityServer(store, live).PutIdentityProvider(as(domain.RoleAdmin), req)
	if err != nil {
		t.Fatalf("PutIdentityProvider: %v", err)
	}
	if _, ok := resp.(openapi.PutIdentityProvider403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want it refused", resp)
	}
	if len(store.stored) != 0 || len(live.added) != 0 {
		t.Error("an area administrator configured an installation-wide sign-in mapping")
	}
}
