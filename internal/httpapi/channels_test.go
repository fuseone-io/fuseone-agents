package httpapi

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
Configuring where runs report.

Two things this has to keep true. A credential is never handed back — the list
says one is stored and nothing else. And the scope a conversation speaks for
comes from what was configured, never from the request: a caller naming a scope
in a delete could otherwise reach a conversation belonging to another.
*/
func TestListChannels_neverHandsBackTheCredential(t *testing.T) {
	t.Parallel()
	s := NewServer(ledger.NewMemory(), "test").
		WithChannels(&channelSpy{listed: []admin.Channel{{
			Name: "acme-slack", Kind: "slack", Enabled: true, HasCredential: true,
		}}}, nil).
		WithChannelListing(&listerSpy{kinds: []string{"slack"}})

	resp, err := s.ListChannels(as(domain.RoleCurator), openapi.ListChannelsRequestObject{})
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	page, ok := resp.(openapi.ListChannels200JSONResponse)
	if !ok {
		t.Fatalf("response = %T", resp)
	}
	if !page.Items[0].HasCredential {
		t.Error("the list does not say a credential is stored")
	}
	// The generated type has no field for it, which is the point: there is
	// nowhere for a token to travel back to a browser.
	if page.Items[0].Kind != "slack" {
		t.Errorf("kind = %q", page.Items[0].Kind)
	}

	// The screen asks the process which vendors it can connect, so it can
	// never offer one the binary has no driver for.
	if len(page.Kinds) != 1 || page.Kinds[0] != "slack" {
		t.Errorf("kinds = %v, want what this binary can build", page.Kinds)
	}
}

type listerSpy struct{ kinds []string }

func (l *listerSpy) Conversations(context.Context, string) ([]channel.Available, error) {
	return nil, nil
}

func (l *listerSpy) Kinds() []string { return l.kinds }

func TestDeleteConversation_scopeComesFromWhatIsStored(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{listed: []admin.Channel{{
		Name: "acme-slack",
		Conversations: []admin.Conversation{{
			ID: "C07", Scope: domain.Scope{Company: "acme", Area: "ops"},
		}},
	}}}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	if _, err := s.DeleteConversation(as(domain.RoleCurator),
		openapi.DeleteConversationRequestObject{Name: "acme-slack", Conversation: "C07"},
	); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}

	if spy.deletedScope.Area != "ops" {
		t.Errorf("deleted in %s, want the scope the conversation was configured in",
			spy.deletedScope)
	}
}

// The reason a test button exists: proving the bot was invited without waiting
// for a run to park.
func TestTestConversation_channelRefuses_saysWhyInItsOwnWords(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{listed: []admin.Channel{{
		Name: "acme-slack",
		Conversations: []admin.Conversation{{ID: "C07",
			Scope: domain.Scope{Company: "acme", Area: "ops"}}},
	}}}
	s := NewServer(ledger.NewMemory(), "test").
		WithChannels(spy, &refusingPoster{err: errors.New("slack: refused: not_in_channel")})

	resp, err := s.TestConversation(as(domain.RoleCurator),
		openapi.TestConversationRequestObject{Name: "acme-slack", Conversation: "C07"})
	if err != nil {
		t.Fatalf("TestConversation: %v", err)
	}

	refused, ok := resp.(openapi.TestConversation400ApplicationProblemPlusJSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the refusal", resp)
	}
	if refused.Detail == nil || *refused.Detail == "" {
		t.Fatal("the refusal says nothing an operator could act on")
	}
}

func TestTestConversation_conversationIsNotConfigured_refusesRatherThanPosting(t *testing.T) {
	t.Parallel()
	poster := &refusingPoster{}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(&channelSpy{}, poster)

	if _, err := s.TestConversation(as(domain.RoleCurator),
		openapi.TestConversationRequestObject{Name: "acme-slack", Conversation: "C99"},
	); err != nil {
		t.Fatalf("TestConversation: %v", err)
	}
	if poster.called {
		t.Error("posted to a conversation nobody configured")
	}
}

type channelSpy struct {
	listed       []admin.Channel
	bound        []admin.ChannelIdentity
	deletedScope domain.Scope
}

func (c *channelSpy) List(context.Context) ([]admin.Channel, error) { return c.listed, nil }

func (c *channelSpy) PutChannel(
	context.Context, admin.Channel, channel.Credentials, domain.UserID,
) error {
	return nil
}

func (c *channelSpy) DeleteChannel(context.Context, string, domain.UserID) error { return nil }

func (c *channelSpy) PutConversation(
	context.Context, string, admin.Conversation, domain.UserID,
) error {
	return nil
}

func (c *channelSpy) DeleteConversation(
	_ context.Context, _ string, scope domain.Scope, _ domain.UserID,
) error {
	c.deletedScope = scope
	return nil
}

type refusingPoster struct {
	err    error
	called bool
}

func (r *refusingPoster) Post(
	context.Context, channel.Conversation, channel.Message,
) (string, error) {
	r.called = true
	if r.err != nil {
		return "", r.err
	}
	return "1.1", nil
}

func (c *channelSpy) Identities(context.Context) ([]admin.ChannelIdentity, error) {
	return c.bound, nil
}

func (c *channelSpy) BindIdentity(
	_ context.Context, id admin.ChannelIdentity, _ domain.UserID,
) error {
	c.bound = append(c.bound, id)
	return nil
}

func (c *channelSpy) UnbindIdentity(context.Context, string, string, domain.UserID) error {
	return nil
}

// Binding is what turns a Slack account into somebody's authority, so the list
// has to say which accounts a channel already trusts — an administrator
// reviewing this needs to see it without opening anything.
func TestListChannels_saysWhichAccountsAreBound(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{
		listed: []admin.Channel{{Name: "acme-slack", Kind: "slack"}},
		bound: []admin.ChannelIdentity{
			{Channel: "acme-slack", Account: "U024", Principal: "usr_ana", Display: "Ana"},
			{Channel: "other", Account: "U999", Principal: "usr_bob"},
		},
	}
	s := NewServer(ledger.NewMemory(), "test").
		WithChannels(spy, nil).
		WithChannelListing(&listerSpy{})

	resp, err := s.ListChannels(as(domain.RoleCurator), openapi.ListChannelsRequestObject{})
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	page, ok := resp.(openapi.ListChannels200JSONResponse)
	if !ok {
		t.Fatalf("response = %T", resp)
	}

	held := page.Items[0].Identities
	if held == nil || len(*held) != 1 {
		t.Fatalf("identities = %v, want only this channel's", held)
	}
	if (*held)[0].Account != "U024" || (*held)[0].Principal != "usr_ana" {
		t.Errorf("identity = %+v", (*held)[0])
	}
}
