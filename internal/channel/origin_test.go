package channel_test

import (
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
Which scope an ask in a conversation belongs to.

The outbound half asks the opposite question and cannot be read backwards. A
run reports to every conversation whose scope contains its own, and that
containment is right for hearing and wrong for asking: a company-wide channel
that hears about every area is reasonable to configure, and one that can start
an agent in every area is a different grant. Reading one map in both directions
would make visibility and action symmetric by accident.
*/

func TestResolve_aConfiguredConversation_answersItsOwnScope(t *testing.T) {
	store, channels := configuredChannels(t)

	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C07-ops", Label: "#ops", Enabled: true,
		Scope: domain.Scope{Company: "acme", Area: "ops"},
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation: %v", err)
	}

	got, err := store.Resolve(t.Context(), "acme-slack", "C07-ops")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Scope != (domain.Scope{Company: "acme", Area: "ops"}) {
		t.Errorf("scope = %+v, want the conversation's own", got.Scope)
	}
}

/*
A conversation nobody configured is answered the same way as one that does not
exist, because a caller learning which channels this installation listens in is
a caller mapping it.
*/
// A conversation somebody switched off starts nothing. It is configuration
// that exists and is not in force, and the outbound half already reads it that
// way — a channel that stopped hearing must not go on being able to ask.
func TestResolve_aConversationSwitchedOff_isRefused(t *testing.T) {
	store, channels := configuredChannels(t)

	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C08-quiet", Label: "#quiet", Enabled: false,
		Scope: domain.Scope{Company: "acme", Area: "ops"},
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation: %v", err)
	}

	if _, err := store.Resolve(t.Context(), "acme-slack", "C08-quiet"); !errors.Is(err, channel.ErrNoConversation) {
		t.Errorf("err = %v, want ErrNoConversation", err)
	}
}

func TestResolve_aConversationNobodyConfigured_isRefused(t *testing.T) {
	store, _ := configuredChannels(t)

	_, err := store.Resolve(t.Context(), "acme-slack", "C99-nobody")
	if !errors.Is(err, channel.ErrNoConversation) {
		t.Errorf("err = %v, want ErrNoConversation", err)
	}
}

func TestWatchFor_aConfiguredSourceAnswersTheAutomation(t *testing.T) {
	store, channels := configuredChannels(t)

	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C07-ops", Label: "#ops", Enabled: true,
		Scope:   domain.Scope{Company: "acme", Area: "ops"},
		Mode:    channel.ConversationWatch,
		Sources: []string{"B-alerts"},
		Agent:   "triagem", RunAs: "usr_opsbot",
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation: %v", err)
	}

	rule, ok, err := store.WatchFor(t.Context(), "acme-slack", "C07-ops",
		channel.Source{Bot: "B-alerts"})
	if err != nil {
		t.Fatalf("WatchFor: %v", err)
	}
	if !ok || rule.Agent != "triagem" || rule.RunAs != "usr_opsbot" {
		t.Fatalf("rule = %+v, ok = %v, want the configured automation", rule, ok)
	}

	_, ok, err = store.WatchFor(t.Context(), "acme-slack", "C07-ops",
		channel.Source{Bot: "B-other"})
	if err != nil {
		t.Fatalf("WatchFor other: %v", err)
	}
	if ok {
		t.Fatal("a message from an unconfigured source matched the watch rule")
	}
}

func TestFor_keepsTheWatchedAgentForOutboundFiltering(t *testing.T) {
	store, channels := configuredChannels(t)

	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C07-ticketito", Label: "#tickets", Enabled: true,
		Scope:   domain.Scope{Company: "acme", Area: "ops"},
		Mode:    channel.ConversationWatch,
		Sources: []string{"B-ticketito"},
		Agent:   "ticketito", RunAs: "usr_opsbot",
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation: %v", err)
	}

	places, err := store.For(t.Context(), domain.Scope{Company: "acme", Area: "ops"})
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	for _, place := range places {
		if place.ID == "C07-ticketito" && place.Agent == "ticketito" {
			return
		}
	}
	t.Fatalf("places = %+v, want the outbound route to keep its watched agent", places)
}

func TestWatchFor_bothModeAlsoAnswersTheAutomation(t *testing.T) {
	store, channels := configuredChannels(t)

	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C07-ops", Label: "#ops", Enabled: true,
		Scope:   domain.Scope{Company: "acme", Area: "ops"},
		Mode:    channel.ConversationBoth,
		Sources: []string{"B-alerts"},
		Agent:   "triagem", RunAs: "usr_opsbot",
		ThreadContext: true,
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation: %v", err)
	}

	rule, ok, err := store.WatchFor(t.Context(), "acme-slack", "C07-ops",
		channel.Source{Bot: "B-alerts"})
	if err != nil {
		t.Fatalf("WatchFor: %v", err)
	}
	if !ok || rule.Agent != "triagem" || rule.RunAs != "usr_opsbot" {
		t.Fatalf("rule = %+v, ok = %v, want the configured automation", rule, ok)
	}

	include, err := store.IncludeThreadContext(t.Context(), "acme-slack", "C07-ops")
	if err != nil {
		t.Fatalf("IncludeThreadContext: %v", err)
	}
	if !include {
		t.Fatal("both mode did not keep the mention-thread context choice")
	}
}

func TestIncludeThreadContext_onlyMentionsConversationsCanChooseIt(t *testing.T) {
	store, channels := configuredChannels(t)

	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C07-alerts", Label: "#alerts", Enabled: true,
		Scope:         domain.Scope{Company: "acme", Area: "ops"},
		ThreadContext: true,
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation mentions: %v", err)
	}
	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C08-watch", Label: "#watch", Enabled: true,
		Scope: domain.Scope{Company: "acme", Area: "ops"},
		Mode:  channel.ConversationWatch, Sources: []string{"B-alerts"},
		Agent: "triagem", RunAs: "usr_opsbot", ThreadContext: true,
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation watch: %v", err)
	}

	include, err := store.IncludeThreadContext(t.Context(), "acme-slack", "C07-alerts")
	if err != nil {
		t.Fatalf("IncludeThreadContext mentions: %v", err)
	}
	if !include {
		t.Fatal("mentions conversation did not keep the thread context choice")
	}
	include, err = store.IncludeThreadContext(t.Context(), "acme-slack", "C08-watch")
	if err != nil {
		t.Fatalf("IncludeThreadContext watch: %v", err)
	}
	if include {
		t.Fatal("watch mode included mention-thread context")
	}
}

func TestPutConversation_watchModeRequiresAuthorityAndSource(t *testing.T) {
	_, channels := configuredChannels(t)

	err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C07-ops", Enabled: true,
		Scope: domain.Scope{Company: "acme", Area: "ops"},
		Mode:  channel.ConversationWatch,
		Agent: "triagem", RunAs: "usr_opsbot",
	}, "usr_ana")
	if !errors.Is(err, admin.ErrNoWatchSource) {
		t.Fatalf("err = %v, want missing source refused", err)
	}
}

func configuredChannels(t *testing.T) (*channel.Configured, *admin.Channels) {
	t.Helper()
	configured, channels, _ := configuredChannelsWithStore(t)
	return configured, channels
}

// configuredChannelsWithStore also hands back the settings store, for the tests
// that have to write a row the administration would never produce. Restore,
// migration and a hand-edited row are how those arrive, and they are what the
// locks on the read side exist for.
func configuredChannelsWithStore(
	t *testing.T,
) (*channel.Configured, *admin.Channels, *settings.Store) {
	t.Helper()
	_, pool := channelStore(t)
	settingsStore := settings.NewStore(pool, nil)
	return channel.NewConfigured(settingsStore),
		admin.NewChannels(pool, settingsStore), settingsStore
}

/*
The same conversation id on two connections is two conversations.

Slack's channel ids and Teams' conversation ids are two namespaces, and nothing
promises they never collide. Resolved by id alone, a message in one workspace
would be governed by a scope somebody configured for another.
*/
func TestResolve_theSameIdOnTwoConnections_resolvesSeparately(t *testing.T) {
	store, channels := configuredChannels(t)

	for _, one := range []struct {
		channel string
		area    domain.AreaID
	}{{"acme-slack", "ops"}, {"acme-teams", "finance"}} {
		if err := channels.PutConversation(t.Context(), one.channel, admin.Conversation{
			ID: "SHARED-ID", Label: "#shared", Enabled: true,
			Scope: domain.Scope{Company: "acme", Area: one.area},
		}, "usr_ana"); err != nil {
			t.Fatalf("PutConversation on %s: %v", one.channel, err)
		}
	}

	slack, err := store.Resolve(t.Context(), "acme-slack", "SHARED-ID")
	if err != nil {
		t.Fatalf("Resolve slack: %v", err)
	}
	teams, err := store.Resolve(t.Context(), "acme-teams", "SHARED-ID")
	if err != nil {
		t.Fatalf("Resolve teams: %v", err)
	}
	if slack.Scope.Area != "ops" || teams.Scope.Area != "finance" {
		t.Errorf("slack = %v, teams = %v, want each its own", slack, teams)
	}
}

/*
A conversation mapped into two scopes is refused, on the way in and on the way
out.

The screen stops the configuration from being made; the reader stops it being
trusted, because a row can also arrive by restore, by migration, or from a
version of the screen that did not check. Taking the first row would make the
governing scope depend on the order a query returned.
*/
func TestPutConversation_alreadySpeakingForAnotherScope_isRefused(t *testing.T) {
	_, channels := configuredChannels(t)

	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C10-double", Enabled: true,
		Scope: domain.Scope{Company: "acme", Area: "ops"},
	}, "usr_ana"); err != nil {
		t.Fatalf("first PutConversation: %v", err)
	}

	err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C10-double", Enabled: true,
		Scope: domain.Scope{Company: "acme", Area: "finance"},
	}, "usr_ana")
	if !errors.Is(err, admin.ErrConversationMapped) {
		t.Errorf("err = %v, want the second mapping refused", err)
	}
}

// Pointing it at the scope it already has is how somebody renames it or
// changes which events it wants, and is not a conflict.
func TestPutConversation_theSameScopeAgain_isAnEdit(t *testing.T) {
	_, channels := configuredChannels(t)

	for _, label := range []string{"#ops", "#ops-alertas"} {
		if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
			ID: "C11-rename", Label: label, Enabled: true,
			Scope: domain.Scope{Company: "acme", Area: "ops"},
		}, "usr_ana"); err != nil {
			t.Fatalf("PutConversation %q: %v", label, err)
		}
	}
}

/*
Which agent a conversation starts.

Read from the same row as the scope and by the same key, because they are the
same decision: an administrator pointed this conversation at an area and at an
agent, and a mention there needs to name neither.
*/
func TestResolve_aConversationBoundToAnAgent_answersIt(t *testing.T) {
	store, channels := configuredChannels(t)

	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C21-bound", Label: "#bound", Enabled: true, Mode: "both",
		Sources: []string{"B-alerts"}, Agent: "triagem", RunAs: "usr_opsbot",
		Scope: domain.Scope{Company: "acme", Area: "ops"},
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation: %v", err)
	}

	got, err := store.Resolve(t.Context(), "acme-slack", "C21-bound")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Agent != "triagem" {
		t.Errorf("agent = %q, want the configured one", got.Agent)
	}
	if got.Mode != channel.ConversationBoth {
		t.Errorf("mode = %q, want it read from the same row", got.Mode)
	}
}

// Nobody chose is not a failure. A conversation open to whatever its scope
// publishes was the only arrangement before this, and it stays one.
func TestResolve_aConversationNobodyBound_answersEmptyAndNoError(t *testing.T) {
	store, channels := configuredChannels(t)

	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C22-unbound", Label: "#unbound", Enabled: true,
		Scope: domain.Scope{Company: "acme", Area: "ops"},
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation: %v", err)
	}

	got, err := store.Resolve(t.Context(), "acme-slack", "C22-unbound")
	if err != nil || got.Agent != "" {
		t.Errorf("Resolve = %+v, %v; want no agent and no error", got, err)
	}
}

// The same id on two connections is two conversations. An agent bound in one
// workspace must not answer for a channel that merely shares an identifier.
func TestResolve_theSameIdOnAnotherConnection_isNotThatBinding(t *testing.T) {
	store, channels := configuredChannels(t)

	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C23-one-connection", Enabled: true, Mode: "both",
		Sources: []string{"B-alerts"}, Agent: "triagem", RunAs: "usr_opsbot",
		Scope: domain.Scope{Company: "acme", Area: "ops"},
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation slack: %v", err)
	}

	_, err := store.Resolve(t.Context(), "acme-teams", "C23-one-connection")
	if !errors.Is(err, channel.ErrNoConversation) {
		t.Errorf("err = %v, want nothing resolved for another connection", err)
	}
}

/*
A conversation binds an agent whatever starts its runs.

The field was born for watched messages and was erased anywhere else, so a
conversation that only takes mentions could not name the agent it is for — the
one arrangement where saying it out loud helps most, because there the person
is typing.
*/
func TestResolve_aMentionsOnlyConversationBoundToAnAgent_answersIt(t *testing.T) {
	store, channels := configuredChannels(t)

	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C24-mentions-bound", Enabled: true, Mode: "mentions", Agent: "triagem",
		Scope: domain.Scope{Company: "acme", Area: "ops"},
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation: %v", err)
	}

	got, err := store.Resolve(t.Context(), "acme-slack", "C24-mentions-bound")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Agent != "triagem" {
		t.Errorf("agent = %q, want the configured one", got.Agent)
	}
}

/*
The principal and the sources still belong to watched messages alone.

A mention runs as the person whose account is bound, so a RunAs kept on a
conversation that never watches anything is a delegation nothing consumes and
an auditor cannot explain. Dropped on the way in, where an operator can still
be told, rather than ignored at the far end.
*/
func TestPutConversation_mentionsOnly_keepsTheAgentAndDropsTheWatchPrincipal(t *testing.T) {
	_, channels := configuredChannels(t)

	if err := channels.PutChannel(t.Context(), admin.Channel{
		Name: "acme-slack", Kind: "slack", Enabled: true,
	}, channel.Credentials{}, "usr_ana"); err != nil {
		t.Fatalf("PutChannel: %v", err)
	}
	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C25-mentions-only", Enabled: true, Mode: "mentions", Agent: "triagem",
		RunAs: "usr_opsbot", Sources: []string{"B-alerts"},
		Scope: domain.Scope{Company: "acme", Area: "ops"},
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation: %v", err)
	}

	got := storedConversation(t, channels, "acme-slack", "C25-mentions-only")
	if got.Agent != "triagem" {
		t.Errorf("agent = %q, want it kept", got.Agent)
	}
	if got.RunAs != "" || len(got.Sources) != 0 {
		t.Errorf("runAs = %q, sources = %v; want both dropped", got.RunAs, got.Sources)
	}
}

func storedConversation(
	t *testing.T, channels *admin.Channels, name, id string,
) admin.Conversation {
	t.Helper()
	channelList, err := channels.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, one := range channelList {
		if one.Name != name {
			continue
		}
		for _, conv := range one.Conversations {
			if conv.ID == id {
				return conv
			}
		}
	}
	t.Fatalf("no conversation %s on %s", id, name)
	return admin.Conversation{}
}

/*
A conversation that is not told about parked runs sends no private ones either.

The flag is outbound, like Wants, and it rides on the same obligation: the fan
-out happens after a conversation has been found owed an announcement about
this event. So a conversation that never hears about parked runs cannot send a
private card about one, and storing the flag as on would be configuration that
describes something the platform does not do.

Zeroed on the way in rather than ignored at the far end, which is how every
other field a mode or a choice does not consume is treated here.
*/
func TestPutConversation_notToldAboutParkedRuns_storesNoDirectApprovals(t *testing.T) {
	_, channels := configuredChannels(t)

	if err := channels.PutChannel(t.Context(), admin.Channel{
		Name: "acme-slack", Kind: "slack", Enabled: true,
	}, channel.Credentials{}, "usr_ana"); err != nil {
		t.Fatalf("PutChannel: %v", err)
	}
	for _, c := range []struct {
		id     string
		wants  []string
		stored bool
	}{
		{"C30-parked", []string{"parked", "failed"}, true},
		{"C31-finished", []string{"finished"}, false},
		// Empty means the defaults, and parked is one of them.
		{"C32-defaults", nil, true},
	} {
		if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
			ID: c.id, Enabled: true, Wants: c.wants, DirectApprovals: true,
			Scope: domain.Scope{Company: "acme", Area: "ops"},
		}, "usr_ana"); err != nil {
			t.Fatalf("PutConversation %s: %v", c.id, err)
		}
		got := storedConversation(t, channels, "acme-slack", c.id)
		if got.DirectApprovals != c.stored {
			t.Errorf("%s wants %v: stored = %v, want %v",
				c.id, c.wants, got.DirectApprovals, c.stored)
		}
	}
}

/*
The runtime reads the choice the administration stored.

Configured.For lists the fields it carries out one by one, so a field added to
the stored shape and not to that list reads as false everywhere — the console
says on, the settings row says on, and nothing is ever sent. It is the quietest
way for this feature to be absent.
*/
func TestFor_theDirectApprovalChoice_reachesTheRuntime(t *testing.T) {
	store, channels := configuredChannels(t)

	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C33-direct", Enabled: true, Wants: []string{"parked"},
		DirectApprovals: true,
		Scope:           domain.Scope{Company: "acme", Area: "ops"},
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation: %v", err)
	}

	places, err := store.For(t.Context(), domain.Scope{Company: "acme", Area: "ops"})
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	for _, place := range places {
		if place.ID == "C33-direct" {
			if !place.DirectApprovals {
				t.Error("the runtime reads the conversation as not telling anybody privately")
			}
			return
		}
	}
	t.Fatalf("places = %+v, want the conversation", places)
}

/*
A conversation for the whole installation hears everything and asks nothing.

That scope contains every company, and containment is right for hearing and
wrong for asking — the asymmetry this file opens with. A room that hears about
every company is a reasonable thing to configure; one that can start an agent in
every company is a different grant entirely.

Today nothing would come of a mention there: no agent can be published at the
installation, and the catalogue query compares the company for equality rather
than containment, so the startable list comes back empty. **Both of those live
in another package and neither says why.** The rule has to be stated here, or
the day somebody teaches that query to read the sentinel as "everything" — a
change that would look correct — this room becomes a start button for every
agent in the installation.
*/
func TestResolve_aConversationForTheWholeInstallation_startsNothing(t *testing.T) {
	store, _, settingsStore := configuredChannelsWithStore(t)

	installationConversation(t, settingsStore, "C40-everywhere", channel.ConversationMentions)

	_, err := store.Resolve(t.Context(), "acme-slack", "C40-everywhere")
	if !errors.Is(err, channel.ErrAnnouncesOnly) {
		t.Fatalf("err = %v, want ErrAnnouncesOnly", err)
	}
}

// And so does any conversation whose mode says it only announces, wherever it
// sits. The scope is one reason to refuse; the mode is the other.
func TestResolve_aConversationThatOnlyAnnounces_startsNothing(t *testing.T) {
	store, channels, _ := configuredChannelsWithStore(t)

	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C41-quiet", Enabled: true, Mode: channel.ConversationAnnounce,
		Scope: domain.Scope{Company: "acme", Area: "ops"},
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation: %v", err)
	}

	_, err := store.Resolve(t.Context(), "acme-slack", "C41-quiet")
	if !errors.Is(err, channel.ErrAnnouncesOnly) {
		t.Fatalf("err = %v, want ErrAnnouncesOnly", err)
	}
}

/*
Two rows for one Slack channel are still reported as ambiguous.

The refusal for an installation row sits after the count, not inside the loop.
Refusing it while searching would answer "this one announces only" and hide the
fact that two rows exist — sending an operator to look at the wrong one.
*/
func TestResolve_theSameIdAtTheInstallationAndAtACompany_isAmbiguous(t *testing.T) {
	store, _, settingsStore := configuredChannelsWithStore(t)

	// Both written directly. The administration refuses to create the pair —
	// that is its own lock — so a state holding both is restored, migrated or
	// hand-edited, which is the state this read has to survive.
	conversationRow(t, settingsStore, "SHARED-EVERYWHERE",
		settings.ScopeArea, domain.Scope{Company: "acme", Area: "ops"},
		channel.ConversationMentions)
	installationConversation(t, settingsStore, "SHARED-EVERYWHERE", channel.ConversationMentions)

	_, err := store.Resolve(t.Context(), "acme-slack", "SHARED-EVERYWHERE")
	if !errors.Is(err, channel.ErrAmbiguousConversation) {
		t.Fatalf("err = %v, want the ambiguity reported", err)
	}
}

/*
And a watched message in such a room writes nothing down.

WatchFor is asked by the door, before the consumer resolves anything. It never
looked at the scope, so an installation row saying "watch" would let any
configured Slack source write an inbox row carrying a configured principal —
refused a sweep later, after the write and the delegation had already travelled.
*/
func TestWatchFor_aConversationForTheWholeInstallation_answersNoRule(t *testing.T) {
	store, _, settingsStore := configuredChannelsWithStore(t)

	installationConversation(t, settingsStore, "C42-watching", channel.ConversationWatch)

	_, ok, err := store.WatchFor(t.Context(), "acme-slack", "C42-watching",
		channel.Source{Bot: "B-alerts"})
	if err != nil {
		t.Fatalf("WatchFor: %v", err)
	}
	if ok {
		t.Error("a conversation for the whole installation answered with a watch rule")
	}
}

/*
A mode this version does not know starts nothing.

The allowlist is right and it was being asked the wrong question: Resolve
normalised the stored value first, and ConversationMode answers anything it
does not recognise with "mentions" — which is the correct answer for a screen
and the opposite of the correct answer for deciding who may start work. A row
saying "a-future-mode", written by a newer version and restored here, came back
as a conversation anybody could start runs from by typing in it.

Empty is the one unknown value that legitimately means mentions, and it is
named on its own. Everything else fails closed.
*/
func TestResolve_aModeThisVersionDoesNotKnow_startsNothing(t *testing.T) {
	store, _, settingsStore := configuredChannelsWithStore(t)

	conversationRow(t, settingsStore, "C43-future", settings.ScopeArea,
		domain.Scope{Company: "acme", Area: "ops"}, "a-future-mode")

	_, err := store.Resolve(t.Context(), "acme-slack", "C43-future")
	if !errors.Is(err, channel.ErrAnnouncesOnly) {
		t.Fatalf("err = %v, want ErrAnnouncesOnly", err)
	}
}

// And it writes nothing down either. WatchFor reads the stored value for the
// same reason, and is asked first.
func TestWatchFor_aModeThisVersionDoesNotKnow_answersNoRule(t *testing.T) {
	store, _, settingsStore := configuredChannelsWithStore(t)

	conversationRow(t, settingsStore, "C44-future", settings.ScopeArea,
		domain.Scope{Company: "acme", Area: "ops"}, "a-future-mode")

	_, ok, err := store.WatchFor(t.Context(), "acme-slack", "C44-future",
		channel.Source{Bot: "B-alerts"})
	if err != nil {
		t.Fatalf("WatchFor: %v", err)
	}
	if ok {
		t.Error("a mode this version does not know answered with a watch rule")
	}
}

/*
Two rows for one conversation answer no watch rule.

Resolve reports the pair as ambiguous, and WatchFor — asked first, by the door —
answered from whichever row the database returned first. Restored with the same
id at an area and at the installation, the area row won and started the agent
under its stored principal, while a mention in the very same conversation was
being refused as ambiguous. Which of the two is in force is not a question row
order may answer.
*/
func TestWatchFor_theSameIdAtTwoScopes_answersNoRule(t *testing.T) {
	store, _, settingsStore := configuredChannelsWithStore(t)

	conversationRow(t, settingsStore, "SHARED-WATCHING",
		settings.ScopeArea, domain.Scope{Company: "acme", Area: "ops"},
		channel.ConversationWatch)
	installationConversation(t, settingsStore, "SHARED-WATCHING", channel.ConversationWatch)

	_, ok, err := store.WatchFor(t.Context(), "acme-slack", "SHARED-WATCHING",
		channel.Source{Bot: "B-alerts"})
	if err != nil {
		t.Fatalf("WatchFor: %v", err)
	}
	if ok {
		t.Error("an ambiguous conversation answered with a watch rule")
	}
}

// installationConversation writes a row the administration will not produce.
// It arrives by restore, by migration, or from a version of the screen that did
// not check — which is exactly what the locks on the read side are for.
func installationConversation(t *testing.T, store *settings.Store, id, mode string) {
	t.Helper()
	conversationRow(t, store, id, settings.ScopeInstallation,
		domain.Scope{Company: domain.Installation}, mode)
}

func conversationRow(
	t *testing.T, store *settings.Store, id string,
	kind settings.ScopeKind, scope domain.Scope, mode string,
) {
	t.Helper()
	value := `{"channel":"acme-slack","mode":"` + mode +
		`","agent":"triagem","runAs":"usr_opsbot","sources":["B-alerts"]}`
	if err := store.Put(t.Context(), settings.Setting{
		ScopeKind: kind, Scope: scope,
		Kind: channel.KindConversation, Name: id,
		Value: []byte(value), Enabled: true, UpdatedBy: "restore",
	}); err != nil {
		t.Fatalf("write the %s row: %v", kind, err)
	}
}

/*
Removing a conversation removes the row that exists.

Where a conversation is stored is chosen in two places — once when it is
written and once when it is deleted — and the two agreeing is what makes a
delete a delete. Disagreeing, the removal matches nothing, reports no error and
answers 204, and the room goes on receiving every run it was configured for.
Nothing says so: the console shows it gone.
*/
func TestDeleteConversation_atEveryScope_theRowIsGone(t *testing.T) {
	store, channels := configuredChannels(t)

	for _, scope := range []domain.Scope{
		{Company: "acme", Area: "ops"},
		{Company: "acme"},
		{Company: domain.Installation},
	} {
		id := "C50-" + string(scope.Company) + "-" + string(scope.Area)
		if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
			ID: id, Enabled: true, Scope: scope,
		}, "usr_ana"); err != nil {
			t.Fatalf("PutConversation %+v: %v", scope, err)
		}
		if !hears(t, store, id, domain.Scope{Company: "acme", Area: "ops"}) {
			t.Fatalf("%+v: the conversation was not configured", scope)
		}

		if err := channels.DeleteConversation(t.Context(), id, scope, "usr_ana"); err != nil {
			t.Fatalf("DeleteConversation %+v: %v", scope, err)
		}
		if hears(t, store, id, domain.Scope{Company: "acme", Area: "ops"}) {
			t.Errorf("%+v: the conversation still receives after being removed", scope)
		}
	}
}

func hears(t *testing.T, store *channel.Configured, id string, run domain.Scope) bool {
	t.Helper()
	places, err := store.For(t.Context(), run)
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	for _, place := range places {
		if place.ID == id {
			return true
		}
	}
	return false
}

/*
A conversation for the whole installation stores nothing about starting runs.

The scope already reached every company before anybody asked it to — the write
only ever refused an empty one — so such a row exists today with a mode saying
"mentions" and, if somebody sent them, an agent, a principal and a list of
sources. The read side refuses all of it, and configuration describing an
inbound path the platform will not take is configuration nobody can trust.

Coerced rather than refused, like every other field a choice does not consume
here. What survives is what the room is for: which events it hears, and whether
it also tells the people who may decide.
*/
func TestPutConversation_atTheInstallationScope_storesNothingAboutStarting(t *testing.T) {
	_, channels := configuredChannels(t)

	for _, mode := range []string{
		channel.ConversationMentions, channel.ConversationWatch,
		channel.ConversationBoth, channel.ConversationAnnounce, "",
	} {
		id := "C51-" + mode
		if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
			ID: id, Enabled: true, Mode: mode,
			Scope:   domain.Scope{Company: domain.Installation},
			Agent:   "triagem",
			RunAs:   "usr_opsbot",
			Sources: []string{"B-alerts"},
			// Kept: it is what the room is for.
			ThreadContext: true, DirectApprovals: true,
			Wants: []string{"parked"},
		}, "usr_ana"); err != nil {
			t.Fatalf("PutConversation %q: %v", mode, err)
		}

		got := storedConversation(t, channels, "acme-slack", id)
		if got.Mode != channel.ConversationAnnounce {
			t.Errorf("mode %q stored as %q, want announce", mode, got.Mode)
		}
		if got.Agent != "" || got.RunAs != "" || len(got.Sources) != 0 || got.ThreadContext {
			t.Errorf("mode %q: stored inbound configuration %+v", mode, got)
		}
		if !got.DirectApprovals {
			t.Errorf("mode %q: the room stopped telling the people who may decide", mode)
		}
	}
}

/*
A mode this version does not know is refused, not quietly rewritten.

The write normalised too, and normalising here is worse than at the read: an
operator editing a conversation on an older console would turn a room a newer
version had set to start nothing into one that starts runs by mention, with the
trail recording an ordinary edit. Empty stays mentions — that is a conversation
configured before modes existed.
*/
func TestPutConversation_aModeThisVersionDoesNotKnow_isRefused(t *testing.T) {
	_, channels := configuredChannels(t)

	err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C53-future", Enabled: true, Mode: "a-future-mode",
		Scope: domain.Scope{Company: "acme", Area: "ops"},
	}, "usr_ana")
	if !errors.Is(err, admin.ErrUnknownMode) {
		t.Fatalf("err = %v, want ErrUnknownMode", err)
	}
}

/*
The installation has no area, and a row claiming one reaches nothing.

Containment short circuits on the sentinel and requires the area to be empty,
so that shape announces to no scope at all while looking configured — the
quietest way to have a room that never says anything.
*/
func TestPutConversation_theInstallationWithAnArea_isRefused(t *testing.T) {
	_, channels := configuredChannels(t)

	err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C52-nowhere", Enabled: true,
		Scope: domain.Scope{Company: domain.Installation, Area: "ops"},
	}, "usr_ana")
	if !errors.Is(err, admin.ErrInstallationArea) {
		t.Fatalf("err = %v, want ErrInstallationArea", err)
	}
}
