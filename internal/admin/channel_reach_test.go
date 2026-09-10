package admin_test

import (
	"context"
	"encoding/json"
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

/*
A binding lives at one position, and reach reads the same one authority does.

PrincipalFor resolves an arriving account by an exact key at the installation
scope, so that is where a binding means anything. Nothing stopped a row for the
same channel and account existing at a company or an area — the settings key
includes the scope, so the two are different rows — and listing a kind returns
every scope of it.

Read without the position, an area row mapping U123 to Ana sits beside the
installation row mapping U123 to Bruno. Ana's approval is then addressed to
U123, which is Bruno's Slack. It grants nothing: the button resolves Bruno and
the Gate checks Bruno. It has already shown Bruno the run, the agent, the area
and the action somebody wanted approved.
*/
func TestAccountsOn_aBindingOutsideTheInstallationScope_cannotAddress(t *testing.T) {
	channels, store := boundChannels(t)
	ctx := context.Background()

	// The row authority reads: this account is Bruno.
	bind(t, channels, "acme-slack", "U123", "usr_bruno")
	// A row at an area claiming the same account is Ana. Only a restore or a
	// hand-edited row puts one here; BindIdentity writes installation-wide.
	if err := store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeArea,
		Scope:     domain.Scope{Company: "acme", Area: "ops"},
		Kind:      admin.KindChannelIdentity,
		Name:      "acme-slack/U123",
		Value: []byte(
			`{"channel":"acme-slack","account":"U123","principal":"usr_ana"}`),
		Enabled: true, UpdatedBy: "restore",
	}); err != nil {
		t.Fatalf("write the area row: %v", err)
	}

	// And a row that says installation while carrying a company. The kind and
	// the scope are separate columns, so nothing keeps them agreeing, and
	// PrincipalFor looks at an exact position rather than at the kind alone.
	if err := store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Scope:     domain.Scope{Company: "acme"},
		Kind:      admin.KindChannelIdentity,
		Name:      "acme-slack/U123",
		Value: []byte(
			`{"channel":"acme-slack","account":"U123","principal":"usr_carla"}`),
		Enabled: true, UpdatedBy: "restore",
	}); err != nil {
		t.Fatalf("write the mislabelled row: %v", err)
	}

	where, err := channels.AccountsOn(ctx, "acme-slack",
		[]domain.UserID{"usr_ana", "usr_bruno", "usr_carla"})
	if err != nil {
		t.Fatalf("AccountsOn: %v", err)
	}
	for _, stranger := range []domain.UserID{"usr_ana", "usr_carla"} {
		if account, ok := where[stranger]; ok {
			t.Errorf("%s is reachable at %q, which is somebody else's account",
				stranger, account)
		}
	}
	if where["usr_bruno"] != "U123" {
		t.Errorf("where = %v, want the installation binding to still address", where)
	}
}

/*
A key naming no account addresses nobody.

`acme-slack/` parses, is enabled, and names a principal; only the half that
says where to send is missing. Answered as reachable, it hands the next stage
the empty conversation the delivery table now refuses outright — a message
attempted against nothing, and a recipient the sweep believes it has told.
*/
func TestAccountsOn_aKeyNamingNoAccount_isNotReachable(t *testing.T) {
	// Blank both ways the write refuses it. BindIdentity rejects an account
	// that is only spaces, so a stored one arrived by restore and names a
	// place no more than an empty string does.
	for _, blank := range []string{"", "   ", "\t"} {
		channels, store := boundChannels(t)
		ctx := context.Background()

		if err := store.Put(ctx, settings.Setting{
			ScopeKind: settings.ScopeInstallation,
			Kind:      admin.KindChannelIdentity,
			Name:      "acme-slack/" + blank,
			Value: []byte(
				`{"channel":"acme-slack","account":"","principal":"usr_ana"}`),
			Enabled: true, UpdatedBy: "restore",
		}); err != nil {
			t.Fatalf("write the malformed row %q: %v", blank, err)
		}

		where, err := channels.AccountsOn(ctx, "acme-slack", []domain.UserID{"usr_ana"})
		if err != nil {
			t.Fatalf("AccountsOn: %v", err)
		}
		if account, ok := where["usr_ana"]; ok {
			t.Errorf("key %q: reachable at %q, want it to reach nobody", blank, account)
		}
	}
}

/*
The trail says when somebody decided a run's facts may go out privately.

Turning this on decides that an approval reaches people in a direct message
rather than only in a room somebody can be added to or removed from. A trail
that records the conversation being configured without saying that is one an
auditor cannot use to answer who decided it, or when.
*/
func TestPutConversation_turningOnDirectApprovals_isRecordedInTheTrail(t *testing.T) {
	pool := freshPool(t)
	channels := admin.NewChannels(pool, settings.NewStore(pool, nil), onlySlack{})
	ctx := context.Background()

	if err := channels.PutConversation(ctx, "acme-slack", admin.Conversation{
		ID: "C-trailed", Enabled: true, Wants: []string{"parked"},
		DirectApprovals: true,
		Scope:           domain.Scope{Company: "acme", Area: "ops"},
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation: %v", err)
	}

	var action, detail string
	if err := pool.QueryRow(ctx, `
		select action, coalesce(detail::text, '')
		from admin_events where target = $1 order by event_id desc limit 1`,
		"C-trailed").Scan(&action, &detail); err != nil {
		t.Fatalf("read trail: %v", err)
	}
	if action != "channel.conversation.configured" {
		t.Fatalf("action = %q, want the conversation configuration", action)
	}
	// Decoded rather than matched as a substring: the column is jsonb and the
	// database chooses its own spacing, so a string comparison would be
	// asserting Postgres's formatting instead of the fact.
	var recorded map[string]any
	if err := json.Unmarshal([]byte(detail), &recorded); err != nil {
		t.Fatalf("decode the trail detail: %v", err)
	}
	if on, ok := recorded["directApprovals"].(bool); !ok || !on {
		t.Errorf("detail = %s, want it to record the private approvals choice", detail)
	}
}
