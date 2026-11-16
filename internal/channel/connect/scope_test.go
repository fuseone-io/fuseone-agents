package connect

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
A connection is found by its name wherever it is stored (NT-019, phase 4
readers). The name is unique across scopes, so the name alone says which row,
and the row is opened under its own scope. Before this, an area's connection
was looked for at the installation only, found nowhere, and every message on it
failed as a configuration nobody wrote.
*/
func TestReply_aConnectionOfAnArea_speaksWithItsOwnBot(t *testing.T) {
	store, _ := stored(t)
	configureAt(t, store, "cx-slack", settings.ScopeArea, domain.Scope{Company: "acme", Area: "cx"})
	drivers, reached := spies(store)

	if err := drivers.Reply(context.Background(), "cx-slack", "C07", "1786.1", "paused"); err != nil {
		t.Fatalf("reply: %v", err)
	}
	if _, ok := reached["xoxb-cx-slack"]; !ok {
		t.Fatalf("reached = %v, want the area's own bot", reached)
	}
}

// A connection at a scope this binary cannot judge is not opened: its token
// would speak for runs it was never meant for.
func TestReply_aConnectionOfNoKnownScope_isNotOpened(t *testing.T) {
	store, _ := stored(t)
	configureAt(t, store, "odd-slack", settings.ScopeArea,
		domain.Scope{Company: domain.Installation, Area: "cx"})
	drivers, reached := spies(store)

	if err := drivers.Reply(context.Background(), "odd-slack", "C07", "1786.1", "paused"); err == nil {
		t.Fatal("reply succeeded on a connection of no known scope")
	}
	if len(reached) != 0 {
		t.Errorf("reached = %v, want no bot built", reached)
	}
}

// configureAt writes a connection at a scope the administration does not write
// to yet.
func configureAt(
	t *testing.T, s *settings.Store, name string, kind settings.ScopeKind, scope domain.Scope,
) {
	t.Helper()
	value, _ := json.Marshal(channel.Connection{Kind: "slack", Workspace: name})
	creds, _ := json.Marshal(channel.Credentials{Token: "xoxb-" + name, Signing: "s3cr3t"})
	if err := s.Put(context.Background(), settings.Setting{
		ScopeKind: kind, Scope: scope, Kind: channel.KindChannel,
		Name: name, Value: value, Secret: string(creds),
		Enabled: true, UpdatedBy: "test",
	}); err != nil {
		t.Fatalf("configure %s: %v", name, err)
	}
}
