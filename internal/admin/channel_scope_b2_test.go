package admin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
A conversation names a connection whose scope contains its own (NT-019, phase
4 writers): acme/cx's conversation may use the platform's connection or
acme/cx's own, never acme/finance's. Judged under the connection's lock, so a
connection moved between the read and the write is judged as it stands.
*/
func TestPutConversation_onAConnectionThatDoesNotContainIt_isRefused(t *testing.T) {
	channels, _ := scopedChannels(t)
	ctx := context.Background()
	finance := domain.Scope{Company: "acme", Area: "finance"}
	if _, err := channels.WriteChannel(ctx, areaWrite("fin-slack", finance)); err != nil {
		t.Fatalf("WriteChannel: %v", err)
	}

	err := channels.PutConversation(ctx, "fin-slack", admin.Conversation{
		ID: "C-cx", Enabled: true, Scope: acmeCX,
	}, "usr_cx")
	if !errors.Is(err, admin.ErrConversationOutsideConnection) || !admin.Invalid(err) {
		t.Fatalf("PutConversation = %v, want refused as outside the connection", err)
	}
	if err := channels.PutConversation(ctx, "fin-slack", admin.Conversation{
		ID: "C-fin", Enabled: true, Scope: finance,
	}, "usr_fin"); err != nil {
		t.Errorf("finance's own conversation on its own connection: %v", err)
	}
}

/*
A binding on a connection is authorized where the connection stands under its
lock, and the binding is written only if that caller is allowed there. The
connection may be anybody's: an area's included.
*/
func TestBindIdentityIf_isJudgedWhereTheConnectionIsStored(t *testing.T) {
	channels, _ := scopedChannels(t)
	ctx := context.Background()
	if _, err := channels.WriteChannel(ctx, areaWrite("cx-slack", acmeCX)); err != nil {
		t.Fatalf("WriteChannel: %v", err)
	}
	var judged domain.Scope
	refuse := func(_ settings.ScopeKind, stored domain.Scope) error {
		judged = stored
		return errNotYours
	}
	id := admin.ChannelIdentity{Channel: "cx-slack", Account: "U1", Principal: "usr_ana"}

	if err := channels.BindIdentityIf(ctx, id, "usr_x", refuse); !errors.Is(err, errNotYours) || judged != acmeCX {
		t.Fatalf("BindIdentityIf = %v judged at %+v, want refused at acme/cx", err, judged)
	}
	if _, bound, _ := channels.PrincipalFor(ctx, "cx-slack", "U1"); bound {
		t.Fatal("a refused binding was stored")
	}
	if err := channels.BindIdentityIf(ctx, id, "usr_cx", nil); err != nil {
		t.Fatalf("BindIdentityIf allowed: %v", err)
	}
	if err := channels.UnbindIdentityIf(ctx, "cx-slack", "U1", "usr_x", refuse); !errors.Is(err, errNotYours) || judged != acmeCX {
		t.Fatalf("UnbindIdentityIf = %v judged at %+v, want refused at acme/cx", err, judged)
	}
	if _, bound, _ := channels.PrincipalFor(ctx, "cx-slack", "U1"); !bound {
		t.Fatal("a refused unbinding removed the binding")
	}
}
