package admin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/settings"
)

/*
The refusal reaches the door, through the pieces the process actually wires.

Every other test here calls the administration directly, so all of them would
stay green if the server were wired to something else — or to an administration
that cannot say which vendors this binary can talk to. That question is the
whole of the guard: answered "nobody knows" it used to mean "anything", and the
line that answers it lives in one place in the wiring, where deleting it breaks
nothing that is watched.

So this one enters where a request enters: the handler, the real administration,
a real database.
*/
func TestPutConversation_throughTheServer_onAVendorWithNoDriver_isRefused(t *testing.T) {
	pool := freshPool(t)
	store := settings.NewStore(pool, testVault(t))
	channels := admin.NewChannels(pool, store, onlySlack{})
	ctx := context.Background()

	if err := store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeInstallation, Scope: domain.Scope{},
		Kind: channel.KindChannel, Name: "teams-someday",
		Value:   []byte(`{"kind":"teams","deliveryMode":"http"}`),
		Enabled: true, UpdatedBy: "restore",
	}); err != nil {
		t.Fatalf("write the connection: %v", err)
	}

	s := httpapi.NewServer(ledger.NewMemory(), "test").WithChannels(channels, nil)
	resp, err := s.PutConversation(curating(), openapi.PutConversationRequestObject{
		Name: "teams-someday", Conversation: "C-mentions",
		Body: &openapi.PutConversationJSONRequestBody{
			Company: "acme", Area: ptr("ops"),
		},
	})
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	// The caller's mistake, not this installation's failure: they asked for an
	// inbound path on a connection nothing here can build.
	if _, ok := resp.(openapi.PutConversation400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want bad request", resp)
	}

	// And nothing was stored, which is the part a 400 does not prove on its own.
	listed, err := channels.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, one := range listed {
		if one.Name == "teams-someday" && len(one.Conversations) > 0 {
			t.Errorf("the conversation was stored anyway: %+v", one.Conversations)
		}
	}
}

func curating() context.Context {
	return auth.WithPrincipal(context.Background(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{
			Scope: domain.Scope{Company: domain.Installation}, Role: domain.RoleAdmin,
		}},
	})
}

/*
And an administration that cannot say what it can talk to vouches for nothing.

Nil used to mean "everything", which is the wrong way for a boundary to fail:
the answer to "which vendors can this binary reach" was allowed to be "nobody
knows", and that read as yes. It is only asked when something inbound is being
configured on a connection that exists — a decision no read-only process makes,
which is why they may pass nil at all.
*/
func TestPutConversation_byAnAdministrationWithNoDrivers_isRefused(t *testing.T) {
	pool := freshPool(t)
	store := settings.NewStore(pool, testVault(t))
	channels := admin.NewChannels(pool, store, nil)
	ctx := context.Background()

	if err := store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeInstallation, Scope: domain.Scope{},
		Kind: channel.KindChannel, Name: "acme-slack",
		Value:   []byte(`{"kind":"slack","deliveryMode":"http"}`),
		Enabled: true, UpdatedBy: "usr_ana",
	}); err != nil {
		t.Fatalf("write the connection: %v", err)
	}

	err := channels.PutConversation(ctx, "acme-slack", admin.Conversation{
		ID: "C-mentions", Enabled: true, Wants: []string{"parked"},
		Scope: domain.Scope{Company: "acme", Area: "ops"},
	}, "usr_ana")
	if !errors.Is(err, admin.ErrConnectionOnlyAnnounces) {
		t.Fatalf("err = %v, want ErrConnectionOnlyAnnounces", err)
	}

	// Announcing still works: the room hears, it just starts nothing.
	if err := channels.PutConversation(ctx, "acme-slack", admin.Conversation{
		ID: "C-reports", Enabled: true, Wants: []string{"parked"},
		Mode:  channel.ConversationAnnounce,
		Scope: domain.Scope{Company: "acme", Area: "ops"},
	}, "usr_ana"); err != nil {
		t.Fatalf("announcing was refused too: %v", err)
	}
}
