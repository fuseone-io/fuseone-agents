package admin_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

/*
The webhooks' signing secret is found by the connection's name, wherever the
connection is stored, and comes back with the scope that connection serves
(NT-019, phase 4 readers). The scope is what the door narrows a click to: a
decision that arrives on an area's app decides only inside that area.
*/
func TestChannelDoor_aConnectionOfAnArea_answersItsSecretAndItsScope(t *testing.T) {
	door, store := sealedDoor(t)
	cx := domain.Scope{Company: "acme", Area: "cx"}
	sealedConnection(t, store, "cx-slack", settings.ScopeArea, cx)

	held, known := door.Secrets(context.Background(), "cx-slack")

	if !known || held.Credentials.Signing != "sign-cx-slack" {
		t.Fatalf("held = %+v, %v; want the area connection's signing secret", held, known)
	}
	if held.Scope != cx {
		t.Errorf("scope = %+v, want acme/cx", held.Scope)
	}
}

// The installation's connection serves every scope, as it does today.
func TestChannelDoor_theInstallationsConnection_servesEveryScope(t *testing.T) {
	door, store := sealedDoor(t)
	sealedConnection(t, store, "acme-slack", settings.ScopeInstallation, domain.Scope{})

	held, known := door.Secrets(context.Background(), "acme-slack")

	if !known || held.Scope != (domain.Scope{Company: domain.Installation}) {
		t.Fatalf("held = %+v, %v; want the installation's scope", held, known)
	}
}

// A connection of no scope this binary knows admits nothing: read as the
// platform's, any click signed by its app would decide anywhere.
func TestChannelDoor_aConnectionOfNoKnownScope_isUnknown(t *testing.T) {
	door, store := sealedDoor(t)
	sealedConnection(t, store, "odd-slack", settings.ScopeArea,
		domain.Scope{Company: domain.Installation, Area: "cx"})

	if held, known := door.Secrets(context.Background(), "odd-slack"); known {
		t.Fatalf("held = %+v, want the connection unknown", held)
	}
}

func sealedDoor(t *testing.T) (*admin.ChannelDoor, *settings.Store) {
	t.Helper()
	pool := freshPool(t)
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	store := settings.NewStore(pool, v)
	return admin.NewChannelDoor(pool, store), store
}

func sealedConnection(
	t *testing.T, store *settings.Store, name string, kind settings.ScopeKind, scope domain.Scope,
) {
	t.Helper()
	creds, _ := json.Marshal(channel.Credentials{Token: "xoxb-" + name, Signing: "sign-" + name})
	if err := store.Put(context.Background(), settings.Setting{
		ScopeKind: kind, Scope: scope, Kind: channel.KindChannel, Name: name,
		Value: []byte(`{"kind":"slack"}`), Secret: string(creds),
		Enabled: true, UpdatedBy: "test",
	}); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
