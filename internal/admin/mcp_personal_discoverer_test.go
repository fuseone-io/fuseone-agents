package admin_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
)

/*
Who a server that takes each person's credential is discovered as (#483,
delivery 3): the person who configured their credential for it last. Every
worker reads the same rows and picks the same person.

"Last" is when the person wrote it, never when the row was written: the worker
rewrites the row on every refresh of a personal grant and a move rewrites every
credential of the server, and either would hand discovery to whoever's token
happened to be refreshed. A row stored before the instant was recorded has its
row time, and the first rewrite by the system copies that time in, so the order
it had survives the rewrite.
*/

func discoverer(t *testing.T, i *admin.Integrations, at domain.Scope, server string) domain.UserID {
	t.Helper()
	who, found, err := i.MCPPersonalDiscoverer(context.Background(), at, server)
	if err != nil {
		t.Fatalf("MCPPersonalDiscoverer: %v", err)
	}
	if !found {
		return ""
	}
	return who
}

func putPersonalGrantAs(t *testing.T, w areaServerWorld, who domain.UserID, grant domain.MCPOAuthGrant) {
	t.Helper()
	if err := w.integrations.PutMCPPersonalCredential(context.Background(), who, cxScope, "github",
		domain.MCPCredentialPatch{OAuth: &grant}); err != nil {
		t.Fatalf("PutMCPPersonalCredential(%s): %v", who, err)
	}
}

// asLegacy makes the person's row one stored before the instant was recorded,
// written at the given time.
func asLegacy(t *testing.T, w areaServerWorld, who domain.UserID, at time.Time) {
	t.Helper()
	asLegacyIn(t, w.pool, who, at)
}

func asLegacyIn(t *testing.T, pool *pgxpool.Pool, who domain.UserID, at time.Time) {
	t.Helper()
	tag, err := pool.Exec(context.Background(), `update settings
		set value = value - 'configuredAt', updated_at = $2
		where kind = 'mcp_user_credential' and value->>'principal' = $1`, string(who), at)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("make %s's row legacy: %v, %d rows", who, err, tag.RowsAffected())
	}
}

func configuredAt(t *testing.T, w areaServerWorld, who domain.UserID) string {
	t.Helper()
	var raw []byte
	if err := w.pool.QueryRow(context.Background(), `select value from settings
		where kind = 'mcp_user_credential' and value->>'principal' = $1`, string(who)).Scan(&raw); err != nil {
		t.Fatalf("read %s's row: %v", who, err)
	}
	var stored struct {
		ConfiguredAt string `json:"configuredAt"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("decode %s's row: %v", who, err)
	}
	return stored.ConfiguredAt
}

func withTwoPeople(t *testing.T) areaServerWorld {
	t.Helper()
	w := withAreaServer(t)
	allowEgress(t, w.integrations, cxScope, copilotOrigin, "https://issuer.example.com")
	putPersonalGrantAs(t, w, "usr_ana", personalGrant("https://issuer.example.com/token"))
	putPersonalGrantAs(t, w, "usr_bia", personalGrant("https://issuer.example.com/token"))
	return w
}

func refreshAna(t *testing.T, w areaServerWorld) {
	t.Helper()
	was := personalGrant("https://issuer.example.com/token")
	next := was
	next.AccessToken = "rotated-access"
	if err := w.integrations.RefreshMCPServerOAuth(context.Background(), admin.MCPOAuthRefresh{
		Server: "github", Scope: cxScope, Principal: "usr_ana", Was: was, Next: next,
	}); err != nil {
		t.Fatalf("RefreshMCPServerOAuth: %v", err)
	}
}

func TestMCPPersonalDiscoverer_isWhoeverConfiguredTheirCredentialLast(t *testing.T) {
	w := withTwoPeople(t)
	if got := discoverer(t, w.integrations, cxScope, "github"); got != "usr_bia" {
		t.Fatalf("discoverer = %q, want usr_bia, who configured last", got)
	}
	// Recorded on the person's write, not left to the row's time: a later
	// rewrite that forgot to keep it would otherwise reorder the row.
	if configuredAt(t, w, "usr_bia") == "" {
		t.Fatal("Bia's write recorded no configuredAt")
	}

	putPersonalGrantAs(t, w, "usr_ana", personalGrant("https://issuer.example.com/token"))
	if got := discoverer(t, w.integrations, cxScope, "github"); got != "usr_ana" {
		t.Errorf("discoverer = %q, want usr_ana, who configured again since", got)
	}
}

func TestMCPPersonalDiscoverer_aRefreshByTheWorker_doesNotHandDiscoveryOver(t *testing.T) {
	w := withTwoPeople(t)

	refreshAna(t, w)

	if got := discoverer(t, w.integrations, cxScope, "github"); got != "usr_bia" {
		t.Errorf("discoverer = %q after Ana's grant was refreshed, want usr_bia still", got)
	}
}

func TestMCPPersonalDiscoverer_legacyRows_keepTheirOrderThroughARefresh(t *testing.T) {
	w := withTwoPeople(t)
	ana := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	asLegacy(t, w, "usr_ana", ana)
	asLegacy(t, w, "usr_bia", ana.Add(time.Hour))
	if got := discoverer(t, w.integrations, cxScope, "github"); got != "usr_bia" {
		t.Fatalf("discoverer of legacy rows = %q, want usr_bia, written last", got)
	}

	refreshAna(t, w)

	if got := discoverer(t, w.integrations, cxScope, "github"); got != "usr_bia" {
		t.Errorf("discoverer = %q after Ana's legacy row was refreshed, want usr_bia still", got)
	}
	// The mechanism: the refresh copied the row's time in, rather than the
	// order surviving by accident.
	if got, err := time.Parse(time.RFC3339Nano, configuredAt(t, w, "usr_ana")); err != nil || !got.Equal(ana) {
		t.Errorf("Ana's configuredAt = %v (%v), want the legacy row's time %v", got, err, ana)
	}
}

// The two principals are listed in the reverse of their order: the rows come
// back sorted by their encoded names, and usr_au's sorts before usr_aa's. A tie
// settled by whichever row came first would go to usr_au.
func TestMCPPersonalDiscoverer_aTie_goesToTheSmallerPrincipal(t *testing.T) {
	w := withAreaServer(t)
	allowEgress(t, w.integrations, cxScope, copilotOrigin, "https://issuer.example.com")
	same := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	for _, who := range []domain.UserID{"usr_aa", "usr_au"} {
		putPersonalGrantAs(t, w, who, personalGrant("https://issuer.example.com/token"))
		asLegacy(t, w, who, same)
	}

	if got := discoverer(t, w.integrations, cxScope, "github"); got != "usr_aa" {
		t.Errorf("discoverer of a tie = %q, want usr_aa", got)
	}
}

func TestMCPPersonalDiscoverer_nobodyConnected_isNotFound(t *testing.T) {
	w := withAreaServer(t)
	if _, found, err := w.integrations.MCPPersonalDiscoverer(context.Background(), cxScope, "github"); found || err != nil {
		t.Errorf("found=%v err=%v, want nobody and no error", found, err)
	}
}

// A credential is for the row it was stored for: another row of the name is
// not discovered as it (NT-019).
func TestMCPPersonalDiscoverer_aCredentialForAnotherRowOfTheName_isNotCounted(t *testing.T) {
	w := withTwoPeople(t)
	if _, err := w.pool.Exec(context.Background(), `update settings
		set value = value - 'serverAt' where kind = 'mcp_user_credential'`); err != nil {
		t.Fatalf("point the credentials at the installation's row: %v", err)
	}

	if _, found, err := w.integrations.MCPPersonalDiscoverer(context.Background(), cxScope, "github"); found || err != nil {
		t.Errorf("found=%v err=%v for the area's row, want nobody", found, err)
	}
}

// Unreadable is a failure to read, never "nobody connected": that would tell
// the console to wait for a person who is already there.
func TestMCPPersonalDiscoverer_aCorruptRow_isAnError(t *testing.T) {
	w := withTwoPeople(t)
	if _, err := w.pool.Exec(context.Background(), `update settings
		set value = '{"server": 7}' where kind = 'mcp_user_credential' and value->>'principal' = 'usr_bia'`); err != nil {
		t.Fatalf("corrupt Bia's row: %v", err)
	}

	_, _, err := w.integrations.MCPPersonalDiscoverer(context.Background(), cxScope, "github")
	if !errors.Is(err, admin.ErrMCPPersonalCredentialCorrupt) {
		t.Errorf("err = %v, want the corrupt row refused", err)
	}
}

// A move rewrites every credential of the server for its new row, all at one
// time: read by row time, every legacy row would tie, and the smaller principal
// would take discovery from whoever had it.
func TestMCPPersonalDiscoverer_legacyRows_keepTheirOrderThroughAMove(t *testing.T) {
	w := withMovableServer(t, true)
	ctx := context.Background()
	for _, who := range []domain.UserID{"usr_ana", "usr_bia"} {
		if err := w.integrations.PutMCPPersonalCredential(ctx, who, platform, movable,
			domain.MCPCredentialPatch{Token: ptr("tok_" + string(who))}); err != nil {
			t.Fatalf("PutMCPPersonalCredential(%s): %v", who, err)
		}
	}
	ana := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	asLegacyIn(t, w.pool, "usr_ana", ana)
	asLegacyIn(t, w.pool, "usr_bia", ana.Add(time.Hour))

	if _, err := w.move(t, admin.MCPServerMove{}); err != nil {
		t.Fatalf("move: %v", err)
	}

	if got := discoverer(t, w.integrations, cxScope, movable); got != "usr_bia" {
		t.Errorf("discoverer = %q after the move, want usr_bia, who configured last", got)
	}
}

// The trail says which credential discovered the server, by ids alone: whose,
// and which worker. Nothing of the credential and nothing the server answered.
func TestRecordMCPDiscoveredOnBehalf_namesThePersonAndTheWorker_byIdsAlone(t *testing.T) {
	w := withAreaServer(t)
	ctx := context.Background()
	if _, err := w.pool.Exec(ctx, `delete from admin_events where action = 'mcp_server.discovered_on_behalf'`); err != nil {
		t.Fatalf("clean: %v", err)
	}

	if err := w.integrations.RecordMCPDiscoveredOnBehalf(ctx, admin.MCPDiscoveredOnBehalf{
		Server: "github", Scope: cxScope, Principal: "usr_ana", Holder: "worker-1",
	}); err != nil {
		t.Fatalf("RecordMCPDiscoveredOnBehalf: %v", err)
	}

	var principal, company, area, target string
	var detail map[string]any
	if err := w.pool.QueryRow(ctx, `select principal_id, company_id, area_id, target, detail
		from admin_events where action = 'mcp_server.discovered_on_behalf'`).
		Scan(&principal, &company, &area, &target, &detail); err != nil {
		t.Fatalf("read the event: %v", err)
	}
	if principal != string(domain.SystemWorker) || company != "acme" || area != "cx" || target != "github" {
		t.Errorf("event by %s at %s/%s on %s, want the worker's, at acme/cx, on github", principal, company, area, target)
	}
	want := map[string]any{"principal": "usr_ana", "holder": "worker-1"}
	if len(detail) != len(want) || detail["principal"] != want["principal"] || detail["holder"] != want["holder"] {
		t.Errorf("detail = %v, want %v and nothing else", detail, want)
	}
}

// A row that reads as JSON but names another person, or nobody, than its key
// is as corrupt as one that does not read: picked, it would discover as
// whoever the value says.
func TestMCPPersonalDiscoverer_aRowWhosePrincipalIsNotItsKeys_isAnError(t *testing.T) {
	for name, principal := range map[string]string{"another person": "usr_cai", "nobody": ""} {
		t.Run(name, func(t *testing.T) {
			w := withTwoPeople(t)
			if _, err := w.pool.Exec(context.Background(), `update settings
				set value = jsonb_set(value, '{principal}', to_jsonb($1::text))
				where kind = 'mcp_user_credential' and value->>'principal' = 'usr_bia'`, principal); err != nil {
				t.Fatalf("rewrite Bia's principal: %v", err)
			}

			_, _, err := w.integrations.MCPPersonalDiscoverer(context.Background(), cxScope, "github")
			if !errors.Is(err, admin.ErrMCPPersonalCredentialCorrupt) {
				t.Errorf("err = %v, want the row refused as corrupt", err)
			}
		})
	}
}

func TestMCPPersonalDiscoverer_aKeyThatNamesNobody_isAnError(t *testing.T) {
	w := withTwoPeople(t)
	if _, err := w.pool.Exec(context.Background(), `update settings
		set name = split_part(name, '.', 1) || '.!', value = jsonb_set(value, '{principal}', '""')
		where kind = 'mcp_user_credential' and value->>'principal' = 'usr_bia'`); err != nil {
		t.Fatalf("rewrite Bia's key: %v", err)
	}

	_, _, err := w.integrations.MCPPersonalDiscoverer(context.Background(), cxScope, "github")
	if !errors.Is(err, admin.ErrMCPPersonalCredentialCorrupt) {
		t.Errorf("err = %v, want the row refused as corrupt", err)
	}
}
