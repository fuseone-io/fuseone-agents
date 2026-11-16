package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/vault"
)

/*
The Socket Mode sweep opens a connection wherever it is stored (NT-019, phase 4
readers). The sweep already listed every scope; the read of the app-level token
looked only at the installation, so an area's connection was a candidate every
thirty seconds and an error every thirty seconds, and never received anything.
*/
func TestSlackSocket_aConnectionOfAnArea_isOpenedWithItsOwnToken(t *testing.T) {
	manager, store := socketManagerOver(t)
	socketConnection(t, store, "cx-slack", settings.ScopeArea, domain.Scope{Company: "acme", Area: "cx"})

	target, ok := manager.target(context.Background(), slackSocketCandidate{name: "cx-slack"})

	if !ok || target.appToken != "xapp-cx-slack" {
		t.Fatalf("target = %+v, %v; want the area's own app-level token", target, ok)
	}
	// The session keeps the scope it was opened with: every event it receives
	// is judged by it, whatever the row says by the time the event arrives.
	if target.scope != (domain.Scope{Company: "acme", Area: "cx"}) {
		t.Errorf("session scope = %+v, want acme/cx", target.scope)
	}
	if got := manager.receiverFor(target); got.Scope != target.scope || got.Channel != "cx-slack" {
		t.Errorf("receiver = %s at %+v, want the session's connection and scope", got.Channel, got.Scope)
	}
}

// A connection of no scope this binary knows is not opened: its events would
// be admitted as the platform's.
func TestSlackSocket_aConnectionOfNoKnownScope_isNotOpened(t *testing.T) {
	manager, store := socketManagerOver(t)
	socketConnection(t, store, "odd-slack", settings.ScopeArea,
		domain.Scope{Company: domain.Installation, Area: "cx"})

	if target, ok := manager.target(context.Background(), slackSocketCandidate{name: "odd-slack"}); ok {
		t.Fatalf("target = %+v, want nothing opened", target)
	}
}

func socketManagerOver(t *testing.T) (*slackSocketManager, *settings.Store) {
	t.Helper()
	pool := testdb.Pool(t)
	if _, err := pool.Exec(context.Background(),
		`delete from settings where kind in ('channel_identity', 'channel_conversation'); delete from settings where kind = 'channel'`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	store := settings.NewStore(pool, v)
	return &slackSocketManager{
		settings: store,
		log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		running:  make(map[string]runningSlackSocket),
	}, store
}

func socketConnection(
	t *testing.T, store *settings.Store, name string, kind settings.ScopeKind, scope domain.Scope,
) {
	t.Helper()
	value, _ := json.Marshal(channel.Connection{Kind: "slack", DeliveryMode: "socket"})
	creds, _ := json.Marshal(channel.Credentials{Token: "xoxb-" + name, AppToken: "xapp-" + name})
	if err := store.Put(context.Background(), settings.Setting{
		ScopeKind: kind, Scope: scope, Kind: channel.KindChannel, Name: name,
		Value: value, Secret: string(creds), Enabled: true, UpdatedBy: "test",
	}); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
