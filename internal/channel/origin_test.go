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

func TestScopeOf_aConfiguredConversation_answersItsOwnScope(t *testing.T) {
	store, channels := configuredChannels(t)

	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C07-ops", Label: "#ops", Enabled: true,
		Scope: domain.Scope{Company: "acme", Area: "ops"},
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation: %v", err)
	}

	got, err := store.ScopeOf(t.Context(), "acme-slack", "C07-ops")
	if err != nil {
		t.Fatalf("ScopeOf: %v", err)
	}
	if got != (domain.Scope{Company: "acme", Area: "ops"}) {
		t.Errorf("scope = %+v, want the conversation's own", got)
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
func TestScopeOf_aConversationSwitchedOff_isRefused(t *testing.T) {
	store, channels := configuredChannels(t)

	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C08-quiet", Label: "#quiet", Enabled: false,
		Scope: domain.Scope{Company: "acme", Area: "ops"},
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation: %v", err)
	}

	if _, err := store.ScopeOf(t.Context(), "acme-slack", "C08-quiet"); !errors.Is(err, channel.ErrNoConversation) {
		t.Errorf("err = %v, want ErrNoConversation", err)
	}
}

func TestScopeOf_aConversationNobodyConfigured_isRefused(t *testing.T) {
	store, _ := configuredChannels(t)

	_, err := store.ScopeOf(t.Context(), "acme-slack", "C99-nobody")
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
	_, pool := channelStore(t)
	settingsStore := settings.NewStore(pool, nil)
	return channel.NewConfigured(settingsStore), admin.NewChannels(pool, settingsStore)
}

/*
The same conversation id on two connections is two conversations.

Slack's channel ids and Teams' conversation ids are two namespaces, and nothing
promises they never collide. Resolved by id alone, a message in one workspace
would be governed by a scope somebody configured for another.
*/
func TestScopeOf_theSameIdOnTwoConnections_resolvesSeparately(t *testing.T) {
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

	slack, err := store.ScopeOf(t.Context(), "acme-slack", "SHARED-ID")
	if err != nil {
		t.Fatalf("ScopeOf slack: %v", err)
	}
	teams, err := store.ScopeOf(t.Context(), "acme-teams", "SHARED-ID")
	if err != nil {
		t.Fatalf("ScopeOf teams: %v", err)
	}
	if slack.Area != "ops" || teams.Area != "finance" {
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
