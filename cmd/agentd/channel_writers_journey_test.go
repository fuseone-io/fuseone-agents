package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/testdb"
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
