package admin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

/*
A connection is stored at the scope its writer names (NT-019, phase 4
writers). Its secret is sealed under that scope, every reader finds it there by
name, and its removal removes that row.
*/

var acmeCX = domain.Scope{Company: "acme", Area: "cx"}

func TestWriteChannel_atAnArea_isStoredAndReadThere(t *testing.T) {
	channels, door := scopedChannels(t)
	ctx := context.Background()

	if _, err := channels.WriteChannel(ctx, areaWrite("cx-slack", acmeCX)); err != nil {
		t.Fatalf("WriteChannel: %v", err)
	}

	held, known := door.Secrets(ctx, "cx-slack")
	if !known || held.Scope != acmeCX || held.Credentials.Token != "xoxb-cx" {
		t.Fatalf("door = %+v, %v; want the token sealed at acme/cx", held, known)
	}
	found, _, err := channels.Named(ctx, "cx-slack")
	if err != nil || found.At != settings.ScopeArea || found.Stored != acmeCX || !found.HasCredential {
		t.Fatalf("Named = %+v, %v; want acme/cx's connection with its token", found, err)
	}
	listed, err := channels.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if one := channelNamed(t, listed, "cx-slack"); one.Stored != acmeCX || !one.HasCredential {
		t.Errorf("listed = %+v, want acme/cx's connection with its token", one)
	}
}

// One name, one connection: a second scope taking a name another holds is
// refused, whichever of the two writes second.
func TestWriteChannel_aNameAnotherScopeHolds_isTaken(t *testing.T) {
	channels, _ := scopedChannels(t)
	ctx := context.Background()
	if _, err := channels.WriteChannel(ctx, areaWrite("shared-slack", acmeCX)); err != nil {
		t.Fatalf("WriteChannel: %v", err)
	}

	for _, at := range []admin.ChannelWrite{
		areaWrite("shared-slack", domain.Scope{Company: "acme", Area: "finance"}),
		installationWrite("shared-slack"),
	} {
		if _, err := channels.WriteChannel(ctx, at); !errors.Is(err, settings.ErrNameTaken) {
			t.Errorf("WriteChannel at %s %+v = %v, want the name taken", at.At, at.Stored, err)
		}
	}
}

func TestRemoveChannel_atAnArea_removesThatRow(t *testing.T) {
	channels, door := scopedChannels(t)
	ctx := context.Background()
	if _, err := channels.WriteChannel(ctx, areaWrite("cx-slack", acmeCX)); err != nil {
		t.Fatalf("WriteChannel: %v", err)
	}

	if err := channels.RemoveChannel(ctx, admin.ChannelRemoval{
		Name: "cx-slack", By: "usr_ana", At: settings.ScopeArea, Stored: acmeCX,
	}); err != nil {
		t.Fatalf("RemoveChannel: %v", err)
	}
	if held, known := door.Secrets(ctx, "cx-slack"); known {
		t.Fatalf("door = %+v, want the connection gone", held)
	}
}

// A connection carries only the conversations inside its scope, so it is not
// stored where it would strand one already attached to its name.
func TestWriteChannel_whereAnAttachedConversationIsOutsideIt_isRefused(t *testing.T) {
	channels, _ := scopedChannels(t)
	ctx := context.Background()
	if err := channels.PutConversation(ctx, "cx-slack", admin.Conversation{
		ID: "C1", Enabled: true, Scope: domain.Scope{Company: "acme", Area: "finance"},
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation: %v", err)
	}

	_, err := channels.WriteChannel(ctx, areaWrite("cx-slack", acmeCX))
	if !errors.Is(err, admin.ErrConversationOutsideConnection) || !admin.Invalid(err) {
		t.Fatalf("WriteChannel = %v, want the stranded conversation refused as invalid", err)
	}
}

func scopedChannels(t *testing.T) (*admin.Channels, *admin.ChannelDoor) {
	t.Helper()
	pool := freshPool(t)
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	store := settings.NewStore(pool, v)
	return admin.NewChannels(pool, store, onlySlack{}), admin.NewChannelDoor(pool, store)
}

func areaWrite(name string, at domain.Scope) admin.ChannelWrite {
	return admin.ChannelWrite{
		Channel:     admin.Channel{Name: name, Kind: "slack", Enabled: true},
		Credentials: channel.Credentials{Token: "xoxb-cx"},
		By:          "usr_ana", At: settings.ScopeArea, Stored: at,
	}
}

func installationWrite(name string) admin.ChannelWrite {
	return admin.ChannelWrite{
		Channel: admin.Channel{Name: name, Kind: "slack", Enabled: true},
		By:      "usr_ana", Governs: true, At: settings.ScopeInstallation,
	}
}

func channelNamed(t *testing.T, listed []admin.Channel, name string) admin.Channel {
	t.Helper()
	for _, one := range listed {
		if one.Name == name {
			return one
		}
	}
	t.Fatalf("no connection %s in %+v", name, listed)
	return admin.Channel{}
}

// A name another scope holds is answered before the precondition: the writer
// is told the name is taken, not that their revision went stale, which would
// send them to reload a connection that is not theirs and try again.
func TestWriteChannel_aNameTakenWithAStaleIfMatch_isTakenFirst(t *testing.T) {
	channels, _ := scopedChannels(t)
	ctx := context.Background()
	if _, err := channels.WriteChannel(ctx, areaWrite("held-slack", acmeCX)); err != nil {
		t.Fatalf("WriteChannel: %v", err)
	}
	stale := `"v1-stale"`
	expect, err := domain.ParseIfMatch(&stale)
	if err != nil {
		t.Fatal(err)
	}
	w := installationWrite("held-slack")
	w.Expect = expect

	if _, err := channels.WriteChannel(ctx, w); !errors.Is(err, settings.ErrNameTaken) {
		t.Fatalf("WriteChannel = %v, want the name taken before the stale revision", err)
	}
}
