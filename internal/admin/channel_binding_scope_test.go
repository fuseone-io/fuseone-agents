package admin_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
The schema's two binding rules hold for a connection wherever it is stored
(NT-019, phase 4 readers, migration 0111). Before, both looked only at the
installation: a binding on an area's connection was refused as having no
connection, and deleting an area's connection left its bindings behind, each
one an account still speaking for somebody on whatever connection next took the
name.
*/
func TestSchema_aBindingOnAnAreasConnection_isStored(t *testing.T) {
	_, store := boundChannels(t)
	pool := freshPool(t)
	areaConnectionRow(t, store, "cx-slack")

	if _, err := pool.Exec(context.Background(), oldBind, "cx-slack/U1"); err != nil {
		t.Fatalf("a binding on an area's connection: %v", err)
	}
}

func TestSchema_deletingAnAreasConnectionWithBindings_isRefused(t *testing.T) {
	_, store := boundChannels(t)
	pool := freshPool(t)
	ctx := context.Background()
	areaConnectionRow(t, store, "cx-kept")
	if _, err := pool.Exec(ctx, oldBind, "cx-kept/U1"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`delete from settings where kind = 'channel' and name = $1`, "cx-kept"); err == nil {
		t.Fatal("an area's connection was deleted with a binding on it")
	}
	// Moved to another scope is gone from where its bindings were made. A
	// sibling area keeps the kind and changes everything else.
	for _, move := range []string{
		`update settings set scope_kind = 'installation', company_id = '', area_id = ''
			where kind = 'channel' and name = $1`,
		`update settings set area_id = 'finance' where kind = 'channel' and name = $1`,
	} {
		if _, err := pool.Exec(ctx, move, "cx-kept"); err == nil {
			t.Fatalf("an area's connection changed scope with a binding on it: %s", move)
		}
	}
}

func areaConnectionRow(t *testing.T, store *settings.Store, name string) {
	t.Helper()
	if err := store.Put(context.Background(), settings.Setting{
		ScopeKind: settings.ScopeArea, Scope: domain.Scope{Company: "acme", Area: "cx"},
		Kind: "channel", Name: name, Value: []byte(`{"kind":"slack"}`),
		Enabled: true, UpdatedBy: "test",
	}); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
