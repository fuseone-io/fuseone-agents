package admin_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
Where a person can be reached, which is not the same question as who a binding
names.

PrincipalFor travels inbound: an account arrives and has to resolve to a
person, and a wrong answer there hands somebody else's authority to whoever
typed. This is outbound, and a wrong answer sends a private message to the
wrong person — different damage, and a different set of things to be careful
about.

It is also not Identities. That is an administrative listing and shows a
binding that is broken or switched off, because an operator sent to fix one
needs to see it. Reach is the opposite: a binding nobody enabled must never
receive anything, and the two answers are allowed to differ.
*/

func TestAccountsOn_namesWhereEachPersonIsReachable(t *testing.T) {
	channels, _ := boundChannels(t)
	ctx := context.Background()

	bind(t, channels, "acme-slack", "U-ana", "usr_ana")
	bind(t, channels, "acme-slack", "U-bruno", "usr_bruno")
	// The same person on another connection. Two workspaces are two
	// namespaces, and a message going out on one must not be addressed by the
	// other's account.
	bind(t, channels, "acme-teams", "T-ana", "usr_ana")

	where, err := channels.AccountsOn(ctx, "acme-slack",
		[]domain.UserID{"usr_ana", "usr_bruno", "usr_never_bound"})
	if err != nil {
		t.Fatalf("AccountsOn: %v", err)
	}
	if len(where) != 2 || where["usr_ana"] != "U-ana" || where["usr_bruno"] != "U-bruno" {
		t.Fatalf("where = %v, want the two Slack accounts", where)
	}
}

// Nobody asked about is nobody answered about. The caller has a list of people
// to reach, and a map carrying the rest of the workspace would be an invitation
// to address somebody nobody chose.
func TestAccountsOn_somebodyNobodyAskedAbout_isNotReturned(t *testing.T) {
	channels, _ := boundChannels(t)

	bind(t, channels, "acme-slack", "U-ana", "usr_ana")
	bind(t, channels, "acme-slack", "U-carla", "usr_carla")

	where, err := channels.AccountsOn(context.Background(), "acme-slack",
		[]domain.UserID{"usr_ana"})
	if err != nil {
		t.Fatalf("AccountsOn: %v", err)
	}
	if _, ok := where["usr_carla"]; ok {
		t.Errorf("where = %v, want only the person asked about", where)
	}
}

/*
A binding switched off is visible and unreachable, and both at once.

BindIdentity only ever writes an enabled row, so this arrives by restore, by
migration, or by a hand-edited row — the same way every other defended-against
state here arrives, and the reason PrincipalFor checks it too. The listing must
keep showing it, or an operator is sent to fix a row the console says does not
exist; reach must refuse it, or somebody switched off keeps receiving private
messages whose button answers that their account is not linked.
*/
func TestAccountsOn_aBindingSwitchedOff_isListedAndNotReachable(t *testing.T) {
	channels, store := boundChannels(t)
	ctx := context.Background()

	if err := store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      admin.KindChannelIdentity,
		Name:      "acme-slack/U-off",
		Value:     []byte(`{"channel":"acme-slack","account":"U-off","principal":"usr_off"}`),
		Enabled:   false, UpdatedBy: "restore",
	}); err != nil {
		t.Fatalf("write the disabled row: %v", err)
	}

	where, err := channels.AccountsOn(ctx, "acme-slack", []domain.UserID{"usr_off"})
	if err != nil {
		t.Fatalf("AccountsOn: %v", err)
	}
	if account, ok := where["usr_off"]; ok {
		t.Errorf("reachable at %q, want a switched-off binding to reach nobody", account)
	}

	listed, err := channels.Identities(ctx)
	if err != nil {
		t.Fatalf("Identities: %v", err)
	}
	shown := false
	for _, one := range listed {
		if one.Account == "U-off" {
			shown = true
		}
	}
	if !shown {
		t.Error("the administrative listing hid a row somebody has to fix")
	}
}

/*
The key names the row; the value does not get a vote.

PrincipalFor grants authority by the key, so a value claiming another channel
or another account is a copy that can only ever disagree with the thing doing
the work. Read from the value, a row keyed `acme-slack/U-real` whose contents
say `old-slack/U-fake` would address a message to an account this connection
does not have.
*/
func TestAccountsOn_aValueDisagreeingWithItsKey_isReadFromTheKey(t *testing.T) {
	channels, store := boundChannels(t)
	ctx := context.Background()

	if err := store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      admin.KindChannelIdentity,
		Name:      "acme-slack/U-real",
		Value: []byte(
			`{"channel":"old-slack","account":"U-fake","principal":"usr_ana"}`),
		Enabled: true, UpdatedBy: "restore",
	}); err != nil {
		t.Fatalf("write the disagreeing row: %v", err)
	}

	where, err := channels.AccountsOn(ctx, "acme-slack", []domain.UserID{"usr_ana"})
	if err != nil {
		t.Fatalf("AccountsOn: %v", err)
	}
	if where["usr_ana"] != "U-real" {
		t.Errorf("where = %v, want the account the key names", where)
	}
}

// A row nobody can read reaches nobody. It is shown by the listing so it can be
// removed; addressing a message with it would be guessing.
func TestAccountsOn_anUnreadableBinding_isNotReachable(t *testing.T) {
	channels, store := boundChannels(t)
	ctx := context.Background()

	if err := store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      admin.KindChannelIdentity,
		Name:      "acme-slack/U404",
		Value:     []byte(`{"channel":"acme-slack","account":"U404"}`),
		Enabled:   true, UpdatedBy: "restore",
	}); err != nil {
		t.Fatalf("write the broken row: %v", err)
	}

	where, err := channels.AccountsOn(ctx, "acme-slack", []domain.UserID{"usr_ana"})
	if err != nil {
		t.Fatalf("AccountsOn: %v", err)
	}
	if len(where) != 0 {
		t.Errorf("where = %v, want nothing reachable", where)
	}
}

/*
One person with two accounts on one connection is reached at the same one every
time.

Nobody has made this configuration yet — the key is channel and account, so
nothing stops it. The choice matters because it is also the idempotency key of
the message: an answer that changed between sweeps would send the same person
the same approval again every thirty seconds.

Written in both insertion orders, because "whatever the store returns first" is
exactly the answer that looks stable on one machine.
*/
func TestAccountsOn_twoAccountsForOnePerson_choosesTheSameOneEitherWay(t *testing.T) {
	first, _ := boundChannels(t)
	bind(t, first, "acme-slack", "U-aaa", "usr_ana")
	bind(t, first, "acme-slack", "U-zzz", "usr_ana")

	second, _ := boundChannels(t)
	bind(t, second, "acme-slack", "U-zzz", "usr_ana")
	bind(t, second, "acme-slack", "U-aaa", "usr_ana")

	ctx := context.Background()
	one, err := first.AccountsOn(ctx, "acme-slack", []domain.UserID{"usr_ana"})
	if err != nil {
		t.Fatalf("AccountsOn: %v", err)
	}
	other, err := second.AccountsOn(ctx, "acme-slack", []domain.UserID{"usr_ana"})
	if err != nil {
		t.Fatalf("AccountsOn: %v", err)
	}
	if one["usr_ana"] != other["usr_ana"] {
		t.Fatalf("chose %q one way and %q the other", one["usr_ana"], other["usr_ana"])
	}
	if one["usr_ana"] != "U-aaa" {
		t.Errorf("chose %q, want the smallest account id", one["usr_ana"])
	}
}

func bind(t *testing.T, channels *admin.Channels, channelName, account, principal string) {
	t.Helper()
	if err := channels.BindIdentity(context.Background(), admin.ChannelIdentity{
		Channel: channelName, Account: account, Principal: domain.UserID(principal),
	}, "usr_admin"); err != nil {
		t.Fatalf("BindIdentity %s/%s: %v", channelName, account, err)
	}
}
