package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/testdb"
)

// What the console does, on the routes it does it on, from an administrator's real
// credential to the machine's next request. Nothing here reaches past a route.
type accountDesk struct {
	t       *testing.T
	session string
	handler http.Handler
	account string
}

// signedIn is the console's credential: a session cookie, and on a write the CSRF cookie
// echoed in a header.
func (d accountDesk) signedIn(method, path string, body any) *httptest.ResponseRecorder {
	d.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		d.t.Fatalf("encode: %v", err)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: d.session})
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "console"})
	req.Header.Set(auth.CSRFHeader, "console")
	rec := httptest.NewRecorder()
	d.handler.ServeHTTP(rec, req)
	return rec
}

func deskFor(t *testing.T, name string) accountDesk {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Pool(t)
	dir := auth.NewPostgres(pool)
	account, person := "svc_"+name, "usr_desk_"+name
	for _, table := range []string{"api_tokens", "sessions", "role_grants", "principals"} {
		if _, err := pool.Exec(ctx, `delete from `+table+` where principal_id = any($1)`,
			[]string{account, person}); err != nil {
			t.Fatalf("clean %s: %v", table, err)
		}
	}
	if _, err := pool.Exec(ctx, `delete from admin_events where target = $1`, account); err != nil {
		t.Fatalf("clean trail: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into principals (principal_id, kind, provider, subject, display)
		values ($1, 'user', 'local', $1, $1)`, person); err != nil {
		t.Fatalf("person: %v", err)
	}
	if err := dir.SetGrants(ctx, person, []domain.Grant{{
		Scope: domain.Scope{Company: domain.Installation}, Role: domain.RoleAdmin,
	}}, "usr_setup"); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}
	session, _, err := dir.CreateSession(ctx, person, "test", "127.0.0.1", time.Now())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	server := publishServer(t, newPublisher()).WithServiceAccounts(dir).WithPeople(dir).
		WithAccounts(auth.NewLocal(pool, dir))
	handler := routed(dir, server)
	desk := accountDesk{t: t, session: session.Secret, handler: handler, account: account}

	desk.expect(http.StatusCreated, http.MethodPost, "/admin/service-accounts",
		map[string]string{"name": name, "description": "routes journey"})
	desk.expect(http.StatusNoContent, http.MethodPut, "/admin/people/"+account+"/grants",
		map[string]any{"grants": []map[string]string{{"company": "acme", "area": "cx", "role": "author"}}})
	return desk
}

func (d accountDesk) expect(code int, method, path string, body any) *httptest.ResponseRecorder {
	d.t.Helper()
	got := d.signedIn(method, path, body)
	if got.Code != code {
		d.t.Fatalf("%s %s = %d, want %d: %s", method, path, got.Code, code, got.Body)
	}
	return got
}

type issuedOnRoute struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

func (d accountDesk) issue(name string) issuedOnRoute {
	d.t.Helper()
	got := d.expect(http.StatusCreated, http.MethodPost, "/admin/service-accounts/"+d.account+"/tokens",
		map[string]any{"name": name, "expiresAt": time.Now().Add(time.Hour)})
	var issued issuedOnRoute
	if err := json.Unmarshal(got.Body.Bytes(), &issued); err != nil {
		d.t.Fatalf("decode: %v", err)
	}
	if issued.Secret == "" {
		d.t.Fatalf("no secret: %s", got.Body)
	}
	return issued
}

// answers is what the machine's next request gets back.
func (d accountDesk) answers(token issuedOnRoute) int {
	d.t.Helper()
	return machine{d.handler, token.Secret}.do(d.t, http.MethodGet, "/agents", nil).Code
}

func TestServiceAccountRoutes_rotating_keepsTheNameAndRevokingStopsOnlyTheOldToken(t *testing.T) {
	desk := deskFor(t, "routes-rotate")
	old := desk.issue("ci")
	replacement := desk.issue("ci")

	if old.ID == replacement.ID {
		t.Fatalf("one name made one token: %s", old.ID)
	}
	if desk.answers(old) != http.StatusOK || desk.answers(replacement) != http.StatusOK {
		t.Fatal("both tokens have to work while the machine moves over")
	}

	desk.expect(http.StatusNoContent, http.MethodDelete,
		"/admin/service-accounts/"+desk.account+"/tokens/"+old.ID, nil)

	if got := desk.answers(old); got != http.StatusUnauthorized {
		t.Errorf("the revoked token's next request = %d, want 401", got)
	}
	if got := desk.answers(replacement); got != http.StatusOK {
		t.Errorf("the replacement's next request = %d, want 200", got)
	}
}

func TestServiceAccountRoutes_disabling_stopsEveryTokenAndEnablingBringsBackOnlyTheUnrevoked(t *testing.T) {
	desk := deskFor(t, "routes-disable")
	revoked, live := desk.issue("old"), desk.issue("live")
	tokens := "/admin/service-accounts/" + desk.account + "/tokens"
	desk.expect(http.StatusNoContent, http.MethodDelete, tokens+"/"+revoked.ID, nil)

	state := "/admin/service-accounts/" + desk.account + "/disabled"
	desk.expect(http.StatusNoContent, http.MethodPut, state, map[string]bool{"disabled": true})

	if got := desk.answers(live); got != http.StatusUnauthorized {
		t.Errorf("a token of a disabled account = %d, want 401", got)
	}
	desk.expect(http.StatusConflict, http.MethodPost, tokens,
		map[string]any{"name": "more", "expiresAt": time.Now().Add(time.Hour)})

	desk.expect(http.StatusNoContent, http.MethodPut, state, map[string]bool{"disabled": false})

	if got := desk.answers(live); got != http.StatusOK {
		t.Errorf("after enabling, the live token = %d, want 200", got)
	}
	if got := desk.answers(revoked); got != http.StatusUnauthorized {
		t.Errorf("after enabling, the revoked token = %d, want 401", got)
	}
}

func TestServiceAccountRoutes_expiry_isRequiredAndEnforced(t *testing.T) {
	desk := deskFor(t, "routes-expiry")
	tokens := "/admin/service-accounts/" + desk.account + "/tokens"
	for name, body := range map[string]map[string]any{
		"absent": {"name": "ci"},
		"past":   {"name": "ci", "expiresAt": time.Now().Add(-time.Minute)},
	} {
		if got := desk.signedIn(http.MethodPost, tokens, body); got.Code != http.StatusBadRequest {
			t.Errorf("%s expiry = %d, want 400: %s", name, got.Code, got.Body)
		}
	}

	issued := desk.issue("ci")
	if _, err := testdb.Pool(t).Exec(context.Background(),
		`update api_tokens set expires_at = now() - interval '1 second' where token_id = $1`, issued.ID); err != nil {
		t.Fatalf("expire: %v", err)
	}
	if got := desk.answers(issued); got != http.StatusUnauthorized {
		t.Errorf("an expired token = %d, want 401", got)
	}
}

func TestServiceAccountRoutes_theSecretIsOnlyInTheResponseThatIssuedIt(t *testing.T) {
	desk := deskFor(t, "routes-secret")
	issued := desk.issue("ci")
	tokens := "/admin/service-accounts/" + desk.account + "/tokens"
	desk.expect(http.StatusNoContent, http.MethodDelete, tokens+"/"+issued.ID, nil)
	desk.expect(http.StatusNoContent, http.MethodPut, "/admin/service-accounts/"+desk.account+"/disabled",
		map[string]bool{"disabled": true})

	listed := desk.expect(http.StatusOK, http.MethodGet, tokens, nil).Body.String()
	if !strings.Contains(listed, issued.ID) || strings.Contains(listed, issued.Secret) {
		t.Errorf("listing = %s", listed)
	}
	accounts := desk.expect(http.StatusOK, http.MethodGet, "/admin/service-accounts", nil).Body.String()
	if !strings.Contains(accounts, desk.account) || strings.Contains(accounts, issued.Secret) {
		t.Errorf("accounts = %s", accounts)
	}

	rows, err := testdb.Pool(t).Query(context.Background(), `
		select action, principal_id, detail::text from admin_events where target = $1 order by event_id`, desk.account)
	if err != nil {
		t.Fatalf("trail: %v", err)
	}
	defer rows.Close()
	var acted []string
	for rows.Next() {
		var action, by, detail string
		if err := rows.Scan(&action, &by, &detail); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if strings.Contains(detail, issued.Secret) {
			t.Errorf("%s carries the secret: %s", action, detail)
		}
		if by != "usr_desk_routes-secret" {
			t.Errorf("%s attributed to %q", action, by)
		}
		acted = append(acted, action)
	}
	want := "service_account.created grants.set service_account.token_issued " +
		"service_account.token_revoked service_account.disabled"
	if got := strings.Join(acted, " "); got != want {
		t.Errorf("trail = %q, want %q", got, want)
	}
}

// The door the token was refused at is also a door for an administrator. A password on
// a machine would survive every revocation on this screen.
func TestServiceAccountRoutes_aPasswordOnAMachine_isRefusedEvenToAnAdministrator(t *testing.T) {
	desk := deskFor(t, "routes-password")

	got := desk.signedIn(http.MethodPut, "/admin/people/"+desk.account+"/password",
		map[string]string{"username": "routes-password", "password": "a-password-long-enough"})

	if got.Code != http.StatusBadRequest {
		t.Fatalf("set a machine's password = %d, want 400: %s", got.Code, got.Body)
	}
}
