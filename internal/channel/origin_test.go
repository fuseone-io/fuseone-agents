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

	got, err := store.ScopeOf(t.Context(), "C07-ops")
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

	if _, err := store.ScopeOf(t.Context(), "C08-quiet"); !errors.Is(err, channel.ErrNoConversation) {
		t.Errorf("err = %v, want ErrNoConversation", err)
	}
}

func TestScopeOf_aConversationNobodyConfigured_isRefused(t *testing.T) {
	store, _ := configuredChannels(t)

	_, err := store.ScopeOf(t.Context(), "C99-nobody")
	if !errors.Is(err, channel.ErrNoConversation) {
		t.Errorf("err = %v, want ErrNoConversation", err)
	}
}

func configuredChannels(t *testing.T) (*channel.Configured, *admin.Channels) {
	t.Helper()
	_, pool := channelStore(t)
	settingsStore := settings.NewStore(pool, nil)
	return channel.NewConfigured(settingsStore), admin.NewChannels(pool, settingsStore)
}
