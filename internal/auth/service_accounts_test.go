package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/testdb"
)

var serviceNow = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

func serviceStore(t *testing.T) (*auth.Postgres, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.Pool(t)
	if _, err := pool.Exec(context.Background(), `
		delete from api_tokens;
		delete from role_grants where principal_id like 'svc_%';
		delete from principals where kind = 'service';
		delete from admin_events where action like 'service_account.%'`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	return auth.NewPostgres(pool), pool
}

func aServiceAccount(t *testing.T, store *auth.Postgres, name string) domain.ServiceAccount {
	t.Helper()
	account, err := store.CreateServiceAccount(context.Background(), name, "publishes from CI", "usr_admin")
	if err != nil {
		t.Fatalf("CreateServiceAccount: %v", err)
	}
	return account
}

func TestIssueServiceToken_authenticatesWithExactlyTheGrantsItWasGiven(t *testing.T) {
	store, _ := serviceStore(t)
	ctx := context.Background()
	account := aServiceAccount(t, store, "ci-publisher")
	granted := []domain.Grant{{
		Scope: domain.Scope{Company: "acme", Area: "cx"}, Role: domain.RoleAuthor,
	}}
	if err := store.SetGrants(ctx, account.ID, granted, "usr_admin"); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}

	issued, err := store.IssueServiceToken(ctx, account.ID, "main", "usr_admin", serviceNow.Add(time.Hour), serviceNow)
	if err != nil {
		t.Fatalf("IssueServiceToken: %v", err)
	}

	principal, err := store.PrincipalByToken(ctx, auth.HashToken(issued.Secret), serviceNow)
	if err != nil {
		t.Fatalf("PrincipalByToken: %v", err)
	}
	if principal.Kind != domain.PrincipalService || string(principal.ID) != account.ID {
		t.Fatalf("principal = %+v", principal)
	}
	if len(principal.Grants) != 1 || principal.Grants[0].Role != domain.RoleAuthor {
		t.Fatalf("grants = %+v, want exactly the one granted", principal.Grants)
	}
	if principal.Can(domain.PermAgentPublish, domain.Scope{Company: "acme", Area: "finance"}) {
		t.Fatal("the token reaches a scope it was not given")
	}
}

func TestPrincipalByToken_refusedOnceRevokedExpiredOrDisabled(t *testing.T) {
	store, _ := serviceStore(t)
	ctx := context.Background()

	refused := map[string]func(t *testing.T) string{
		"revoked": func(t *testing.T) string {
			a := aServiceAccount(t, store, "revoked")
			issued, err := store.IssueServiceToken(ctx, a.ID, "t", "usr_admin", serviceNow.Add(time.Hour), serviceNow)
			if err != nil {
				t.Fatalf("Issue: %v", err)
			}
			if err := store.RevokeServiceToken(ctx, a.ID, issued.ID, "usr_admin", serviceNow); err != nil {
				t.Fatalf("Revoke: %v", err)
			}
			return issued.Secret
		},
		"disabled": func(t *testing.T) string {
			a := aServiceAccount(t, store, "disabled")
			issued, err := store.IssueServiceToken(ctx, a.ID, "t", "usr_admin", serviceNow.Add(time.Hour), serviceNow)
			if err != nil {
				t.Fatalf("Issue: %v", err)
			}
			if err := store.SetServiceAccountDisabled(ctx, a.ID, true, "usr_admin", serviceNow); err != nil {
				t.Fatalf("Disable: %v", err)
			}
			return issued.Secret
		},
	}
	for name, setup := range refused {
		secret := setup(t)
		if _, err := store.PrincipalByToken(ctx, auth.HashToken(secret), serviceNow); !errors.Is(err, auth.ErrBadCredential) {
			t.Errorf("%s: err = %v, want refused", name, err)
		}
	}

	a := aServiceAccount(t, store, "lapsing")
	issued, err := store.IssueServiceToken(ctx, a.ID, "t", "usr_admin", serviceNow.Add(time.Hour), serviceNow)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := store.PrincipalByToken(ctx, auth.HashToken(issued.Secret), serviceNow.Add(time.Hour)); !errors.Is(err, auth.ErrExpired) {
		t.Errorf("expired: err = %v", err)
	}
}

// Rotation is issuing beside the old one and revoking it later, and a person will
// reuse the name. The id used to be derived from it, so the second one failed.
func TestIssueServiceToken_theSameNameAgain_isANewToken(t *testing.T) {
	store, _ := serviceStore(t)
	ctx := context.Background()
	a := aServiceAccount(t, store, "rotating")

	first, err := store.IssueServiceToken(ctx, a.ID, "main", "usr_admin", serviceNow.Add(time.Hour), serviceNow)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := store.IssueServiceToken(ctx, a.ID, "main", "usr_admin", serviceNow.Add(time.Hour), serviceNow)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.ID == second.ID || first.Secret == second.Secret {
		t.Fatalf("tokens are not distinct: %q %q", first.ID, second.ID)
	}
	if err := store.RevokeServiceToken(ctx, a.ID, first.ID, "usr_admin", serviceNow); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := store.PrincipalByToken(ctx, auth.HashToken(second.Secret), serviceNow); err != nil {
		t.Fatalf("the other token stopped working: %v", err)
	}
}

func TestIssueServiceToken_refuses(t *testing.T) {
	store, _ := serviceStore(t)
	ctx := context.Background()
	a := aServiceAccount(t, store, "refusing")
	off := aServiceAccount(t, store, "turned-off")
	if err := store.SetServiceAccountDisabled(ctx, off.ID, true, "usr_admin", serviceNow); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if _, err := testdb.Pool(t).Exec(ctx, `
		insert into principals (principal_id, kind, provider, subject, display)
		values ('usr_person', 'user', 'test', 'usr_person', 'A Person') on conflict do nothing`); err != nil {
		t.Fatalf("seed person: %v", err)
	}

	cases := []struct {
		name    string
		account string
		expires time.Time
		want    error
	}{
		{"an expiry already past", a.ID, serviceNow, auth.ErrTokenExpiry},
		{"no expiry at all", a.ID, time.Time{}, auth.ErrTokenExpiry},
		{"a disabled account", off.ID, serviceNow.Add(time.Hour), auth.ErrServiceAccountDisabled},
		{"nobody", "svc_nobody", serviceNow.Add(time.Hour), auth.ErrNoServiceAccount},
		// A token on a person would be a way to act as them with no sign-in.
		{"a person", "usr_person", serviceNow.Add(time.Hour), auth.ErrNoServiceAccount},
	}
	for _, tc := range cases {
		if _, err := store.IssueServiceToken(ctx, tc.account, "t", "usr_admin", tc.expires, serviceNow); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestRevokeServiceToken_ofAnotherAccount_isAbsent(t *testing.T) {
	store, _ := serviceStore(t)
	ctx := context.Background()
	mine, theirs := aServiceAccount(t, store, "mine"), aServiceAccount(t, store, "theirs")
	issued, err := store.IssueServiceToken(ctx, theirs.ID, "t", "usr_admin", serviceNow.Add(time.Hour), serviceNow)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if err := store.RevokeServiceToken(ctx, mine.ID, issued.ID, "usr_admin", serviceNow); !errors.Is(err, auth.ErrNoServiceToken) {
		t.Fatalf("err = %v, want absent", err)
	}
	if _, err := store.PrincipalByToken(ctx, auth.HashToken(issued.Secret), serviceNow); err != nil {
		t.Fatalf("the token was revoked anyway: %v", err)
	}
}

func TestCreateServiceAccount_refusesABadOrTakenName(t *testing.T) {
	store, _ := serviceStore(t)
	ctx := context.Background()
	aServiceAccount(t, store, "taken")

	if _, err := store.CreateServiceAccount(ctx, "taken", "", "usr_admin"); !errors.Is(err, auth.ErrServiceAccountTaken) {
		t.Errorf("taken: err = %v", err)
	}
	for _, name := range []string{"", "A", "Has Space", "-leading", "ümlaut"} {
		if _, err := store.CreateServiceAccount(ctx, name, "", "usr_admin"); !errors.Is(err, auth.ErrServiceAccountName) {
			t.Errorf("%q: err = %v", name, err)
		}
	}
}

// Listing is how a person finds what to revoke, and it must never be how a secret
// comes back. Only the hash is ever stored.
func TestServiceTokens_listWithoutTheSecret_andStoreOnlyItsHash(t *testing.T) {
	store, pool := serviceStore(t)
	ctx := context.Background()
	a := aServiceAccount(t, store, "listed")
	issued, err := store.IssueServiceToken(ctx, a.ID, "main", "usr_admin", serviceNow.Add(time.Hour), serviceNow)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	tokens, err := store.ServiceTokens(ctx, a.ID)
	if err != nil {
		t.Fatalf("ServiceTokens: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != issued.ID || tokens[0].Name != "main" {
		t.Fatalf("tokens = %+v", tokens)
	}

	var stored int
	if err := pool.QueryRow(ctx, `select count(*) from api_tokens where token_hash = $1`,
		auth.HashToken(issued.Secret)).Scan(&stored); err != nil || stored != 1 {
		t.Fatalf("hash rows = %d, %v", stored, err)
	}
	var plain int
	if err := pool.QueryRow(ctx, `select count(*) from api_tokens t where t::text like '%' || $1 || '%'`,
		issued.Secret).Scan(&plain); err != nil || plain != 0 {
		t.Fatalf("rows holding the secret = %d, %v", plain, err)
	}
}

// What was created, by whom and when has to be on the trail, and the trail is read by
// more people than may hold the secret.
func TestServiceAccounts_everyActIsOnTheTrail_withoutTheSecret(t *testing.T) {
	store, pool := serviceStore(t)
	ctx := context.Background()
	a := aServiceAccount(t, store, "trailed")
	issued, err := store.IssueServiceToken(ctx, a.ID, "main", "usr_admin", serviceNow.Add(time.Hour), serviceNow)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if err := store.RevokeServiceToken(ctx, a.ID, issued.ID, "usr_admin", serviceNow); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if err := store.SetServiceAccountDisabled(ctx, a.ID, true, "usr_admin", serviceNow); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if err := store.SetServiceAccountDisabled(ctx, a.ID, false, "usr_admin", serviceNow); err != nil {
		t.Fatalf("Enable: %v", err)
	}

	for _, action := range []string{
		"service_account.created", "service_account.token_issued", "service_account.token_revoked",
		"service_account.disabled", "service_account.enabled",
	} {
		var by string
		if err := pool.QueryRow(ctx, `
			select principal_id from admin_events where action = $1 and target = $2`,
			action, a.ID).Scan(&by); err != nil || by != "usr_admin" {
			t.Errorf("%s: by %q, %v", action, by, err)
		}
	}
	var leaked int
	if err := pool.QueryRow(ctx, `select count(*) from admin_events e where e::text like '%' || $1 || '%'`,
		issued.Secret).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("events holding the secret = %d, %v", leaked, err)
	}
}

// Giving a machine access is what an auditor asks about first.
func TestSetGrants_isOnTheTrail(t *testing.T) {
	store, pool := serviceStore(t)
	ctx := context.Background()
	a := aServiceAccount(t, store, "granted")
	if _, err := pool.Exec(ctx, `delete from admin_events where action = 'grants.set' and target = $1`, a.ID); err != nil {
		t.Fatalf("clean: %v", err)
	}

	if err := store.SetGrants(ctx, a.ID, []domain.Grant{{
		Scope: domain.Scope{Company: "acme", Area: "cx"}, Role: domain.RoleAuthor,
	}}, "usr_admin"); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `
		select count(*) from admin_events
		where action = 'grants.set' and target = $1 and principal_id = 'usr_admin'
		  and detail->'grants' @> '[{"role":"author","company":"acme","area":"cx"}]'`, a.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("grant events = %d, %v", count, err)
	}
}

// Nothing is recorded as granted to somebody who does not exist: the trail would say
// access changed when nothing did, and an empty list used to do exactly that.
func TestSetGrants_onNobody_isAbsent_andLeavesNoEvent(t *testing.T) {
	store, pool := serviceStore(t)
	ctx := context.Background()

	cases := map[string][]domain.Grant{
		"an empty list": nil,
		"a grant":       {{Scope: domain.Scope{Company: "acme", Area: "cx"}, Role: domain.RoleAuthor}},
	}
	for name, grants := range cases {
		err := store.SetGrants(ctx, "svc_nobody", grants, "usr_admin")
		if !errors.Is(err, auth.ErrNoPrincipal) {
			t.Errorf("%s: err = %v, want absent", name, err)
		}
	}

	var events int
	if err := pool.QueryRow(ctx, `
		select count(*) from admin_events where action = 'grants.set' and target = 'svc_nobody'`).Scan(&events); err != nil || events != 0 {
		t.Fatalf("events about nobody = %d, %v", events, err)
	}
}

// Anchored: revoking everything is a real act on somebody real.
func TestSetGrants_anEmptyListOnSomebody_isStoredAndRecorded(t *testing.T) {
	store, pool := serviceStore(t)
	ctx := context.Background()
	a := aServiceAccount(t, store, "emptied")
	if err := store.SetGrants(ctx, a.ID, []domain.Grant{{
		Scope: domain.Scope{Company: "acme", Area: "cx"}, Role: domain.RoleAuthor,
	}}, "usr_admin"); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}

	if err := store.SetGrants(ctx, a.ID, nil, "usr_admin"); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}

	var held, events int
	if err := pool.QueryRow(ctx, `select count(*) from role_grants where principal_id = $1`, a.ID).Scan(&held); err != nil || held != 0 {
		t.Fatalf("grants held = %d, %v", held, err)
	}
	if err := pool.QueryRow(ctx, `select count(*) from admin_events where action = 'grants.set' and target = $1`, a.ID).Scan(&events); err != nil || events != 2 {
		t.Fatalf("events = %d, %v; want 2", events, err)
	}
}

func TestCreateServiceAccount_acceptsTheShortestNameTheContractAllows(t *testing.T) {
	store, _ := serviceStore(t)

	if _, err := store.CreateServiceAccount(context.Background(), "a", "", "usr_admin"); err != nil {
		t.Fatalf("a one-letter name: %v", err)
	}
}
