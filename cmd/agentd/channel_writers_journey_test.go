package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/vault"
)

/*
An area's administrator connects the area's own Slack app, from the request to
the readers (NT-019, phase 4 writers): stored at the area, read back at the
area, invisible to a sibling, found by the webhooks' door and by the Socket
Mode sweep under the area's scope, and refused a name another scope holds.
*/
func TestChannelWriters_anAreasAdministrator_connectsTheAreasOwnApp(t *testing.T) {
	pool := testdb.Pool(t)
	w := readBackWorldOn(t, pool)
	clean := `delete from settings where kind = 'channel_identity'; delete from settings where kind in ('channel_conversation', 'channel')`
	if _, err := pool.Exec(t.Context(), clean); err != nil {
		t.Fatalf("clean: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(t.Context(), clean) })
	const path = "/admin/channels/cx-slack"
	body := map[string]any{
		"scopeKind": "area", "company": "acme", "area": "cx",
		"kind": "slack", "deliveryMode": "socket", "token": "xoxb-cx", "appToken": "xapp-cx",
	}

	if got := w.call(t, etagCall{method: http.MethodPut, path: path, at: journeyCX, body: body}); got.Code != http.StatusNoContent {
		t.Fatalf("PUT at the area = %d: %s", got.Code, got.Body)
	}
	read := w.call(t, etagCall{method: http.MethodGet, path: path, at: journeyCX})
	var got struct {
		ScopeKind, Company, Area string
		Editable                 bool
	}
	_ = json.Unmarshal(read.Body.Bytes(), &got)
	if read.Code != http.StatusOK || got.ScopeKind != "area" || got.Area != "cx" || !got.Editable {
		t.Fatalf("GET = %d %+v: %s", read.Code, got, read.Body)
	}
	if sibling := w.call(t, etagCall{method: http.MethodGet, path: path, at: journeyOps}); sibling.Code != http.StatusForbidden {
		t.Errorf("GET from acme/ops = %d, want refused", sibling.Code)
	}

	// The readers find it where it was written, under the area's scope.
	held, known := admin.NewChannelDoor(pool, w.store).Secrets(t.Context(), "cx-slack")
	if !known || held.Scope != journeyCX {
		t.Errorf("the webhooks' door = %+v, %v; want acme/cx's connection", held, known)
	}
	manager := &slackSocketManager{settings: w.store, running: map[string]runningSlackSocket{}}
	if target, ok := manager.target(t.Context(), slackSocketCandidate{name: "cx-slack"}); !ok || target.scope != journeyCX || target.appToken != "xapp-cx" {
		t.Errorf("the socket sweep = %+v, %v; want acme/cx's session", target, ok)
	}

	if taken := w.call(t, etagCall{method: http.MethodPut, path: path, at: journeyOps, body: map[string]any{
		"scopeKind": "area", "company": "acme", "area": "ops", "kind": "slack",
	}}); taken.Code != http.StatusConflict {
		t.Errorf("PUT of the same name at acme/ops = %d, want 409", taken.Code)
	}
	if got := w.call(t, etagCall{method: http.MethodDelete, path: path, at: journeyCX}); got.Code != http.StatusNoContent {
		t.Fatalf("DELETE at the area = %d: %s", got.Code, got.Body)
	}
}

/*
The area's administrator then attaches the area's room and binds the area's
people, from the request to the rows (NT-019, phase 4 writers). A room of a
sibling area is not theirs to attach; a person whose only grant is over the
area, at the company, is not one of the area's people.
*/
func TestChannelWriters_anAreasAdministrator_attachesRoomsAndBindsTheAreasPeople(t *testing.T) {
	pool := testdb.Pool(t)
	w := channelWorldWithPeople(t, pool)
	clean := `delete from settings where kind = 'channel_identity'; delete from settings where kind in ('channel_conversation', 'channel')`
	if _, err := pool.Exec(t.Context(), clean); err != nil {
		t.Fatalf("clean: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(t.Context(), clean) })
	people := auth.NewPostgres(pool)
	inside := personWith(t, people, "journey-inside", domain.Grant{Scope: journeyCX, Role: domain.RoleApprover})
	over := personWith(t, people, "journey-over", domain.Grant{Scope: domain.Scope{Company: "acme"}, Role: domain.RoleApprover})

	const conn = "/admin/channels/cx-room-slack"
	if got := w.call(t, etagCall{method: http.MethodPut, path: conn, at: journeyCX, body: map[string]any{
		"scopeKind": "area", "company": "acme", "area": "cx", "kind": "slack", "token": "xoxb-cx",
	}}); got.Code != http.StatusNoContent {
		t.Fatalf("PUT the connection = %d: %s", got.Code, got.Body)
	}
	if got := w.call(t, etagCall{method: http.MethodPut, path: conn + "/conversations/C-cx", at: journeyCX,
		body: map[string]any{"company": "acme", "area": "cx"}}); got.Code != http.StatusNoContent {
		t.Fatalf("PUT acme/cx's room = %d: %s", got.Code, got.Body)
	}
	if got := w.call(t, etagCall{method: http.MethodPut, path: conn + "/conversations/C-ops", at: journeyCX,
		body: map[string]any{"company": "acme", "area": "ops"}}); got.Code != http.StatusForbidden {
		t.Errorf("PUT acme/ops's room = %d, want refused", got.Code)
	}
	if got := w.call(t, etagCall{method: http.MethodPut, path: conn + "/identities/U-inside", at: journeyCX,
		body: map[string]any{"principal": inside}}); got.Code != http.StatusNoContent {
		t.Fatalf("bind a person inside acme/cx = %d: %s", got.Code, got.Body)
	}
	if got := w.call(t, etagCall{method: http.MethodPut, path: conn + "/identities/U-over", at: journeyCX,
		body: map[string]any{"principal": over}}); got.Code != http.StatusBadRequest {
		t.Errorf("bind a person over acme/cx = %d, want refused", got.Code)
	}
	who, bound, err := admin.NewChannelDoor(pool, w.store).PrincipalFor(t.Context(), "cx-room-slack", "U-inside")
	if err != nil || !bound || string(who) != inside {
		t.Errorf("the door's binding = %s, %v, %v; want the person inside", who, bound, err)
	}
}

func channelWorldWithPeople(t *testing.T, pool *pgxpool.Pool) *readBackWorld {
	t.Helper()
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	store := settings.NewStore(pool, v)
	api := httpapi.NewServer(ledger.NewMemory(), "test").
		WithChannels(admin.NewChannels(pool, store, readBackDrivers{}), nil).
		WithPeople(auth.NewPostgres(pool))
	return &readBackWorld{route: apiHandlerFor(api), store: store}
}

func personWith(t *testing.T, people *auth.Postgres, subject string, grant domain.Grant) string {
	t.Helper()
	id, err := people.UpsertPrincipal(t.Context(), "journey", subject, subject, subject+"@example.com")
	if err != nil {
		t.Fatalf("UpsertPrincipal: %v", err)
	}
	if err := people.SetGrants(t.Context(), id, []domain.Grant{grant}, "journey"); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}
	return id
}
