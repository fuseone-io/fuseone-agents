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
	"github.com/fuseone/agents/internal/spec"
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

/*
The door decides the caller's authority; the administration decides whether it
was needed.

Read here and acted on there, because between the two a room can be attached:
the check that matters is taken under the connection's lock, beside the write,
and this end's job is only to make sure the answer travels and the refusal comes
back as a refusal.
*/
func TestDeleteChannel_theAdministrationRefuses_isForbiddenRatherThanAnError(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{refuse: admin.ErrInstallationAuthority}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, err := s.DeleteChannel(as(domain.RoleCurator),
		openapi.DeleteChannelRequestObject{Name: "acme-slack"})
	if err != nil {
		t.Fatalf("DeleteChannel: %v", err)
	}
	if _, ok := resp.(openapi.DeleteChannel403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want forbidden", resp)
	}
	if spy.governs {
		t.Error("a curator was reported as governing the installation")
	}
}

func TestPutChannel_theAdministrationRefuses_isForbiddenRatherThanBadRequest(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{refuse: admin.ErrInstallationAuthority}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, err := s.PutChannel(as(domain.RoleCurator), openapi.PutChannelRequestObject{
		Name: "acme-slack", Body: &openapi.PutChannelJSONRequestBody{Kind: "slack"},
	})
	if err != nil {
		t.Fatalf("PutChannel: %v", err)
	}
	// Not a 400. The configuration is fine; the caller is not the one who may
	// make it, and telling somebody their request was malformed sends them to
	// fix a field.
	if _, ok := resp.(openapi.PutChannel403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want forbidden", resp)
	}
}

// And whoever governs the installation is reported as governing it, so the
// administration lets the write through.
func TestPutChannel_byWhoGovernsTheInstallation_saysSo(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, err := s.PutChannel(asInstallation(domain.RoleAdmin), openapi.PutChannelRequestObject{
		Name: "acme-slack", Body: &openapi.PutChannelJSONRequestBody{Kind: "slack"},
	})
	if err != nil {
		t.Fatalf("PutChannel: %v", err)
	}
	if _, ok := resp.(openapi.PutChannel204Response); !ok {
		t.Fatalf("response = %T, want accepted", resp)
	}
	if !spy.governs {
		t.Error("authority over the installation never reached the administration")
	}
}

/*
A failure is not a refusal.

Everything that was not one named sentinel became "bad request", so a lock that
could not be taken, a database that was away or a vault that would not open came
back as 400 — sending somebody to fix a field that was never wrong, with the
operational text in the reply.
*/
func TestPutChannel_theStoreFails_isReportedRatherThanCalledInvalid(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{refuse: errors.New("configuration store unavailable")}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, err := s.PutChannel(asInstallation(domain.RoleAdmin), openapi.PutChannelRequestObject{
		Name: "acme-slack", Body: &openapi.PutChannelJSONRequestBody{Kind: "slack"},
	})
	if err == nil {
		t.Fatalf("response = %T, want the failure reported", resp)
	}
}

// And a configuration the caller really did get wrong still says so.
func TestPutChannel_aConfigurationTheCallerGotWrong_isABadRequest(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{refuse: admin.ErrNoChannelKind}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, err := s.PutChannel(asInstallation(domain.RoleAdmin), openapi.PutChannelRequestObject{
		Name: "acme-slack", Body: &openapi.PutChannelJSONRequestBody{Kind: "slack"},
	})
	if err != nil {
		t.Fatalf("PutChannel: %v", err)
	}
	if _, ok := resp.(openapi.PutChannel400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want bad request", resp)
	}
}

func TestPutConversation_theStoreFails_isReportedRatherThanCalledInvalid(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{refuse: errors.New("configuration store unavailable")}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	if _, err := s.PutConversation(as(domain.RoleCurator), watchConversation("usr_ana")); err == nil {
		t.Fatal("a store failure was answered as a bad request")
	}
}

/*
A mode this version does not know crosses the boundary as it is stored.

The administration keeps it and the console refuses to draw it, and between
them is the reply that used to normalise: read as "mentions" there, an
unrelated edit saved from that reading turned a room that starts nothing into
one anybody can start runs from. Each end was held by its own test and the
seam between them by none.
*/
func TestListChannels_aModeThisVersionDoesNotKnow_travelsAsItIsStored(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{listed: []admin.Channel{{
		Name: "acme-slack",
		Conversations: []admin.Conversation{{
			ID: "C07", Mode: "a-future-mode",
			Scope: domain.Scope{Company: "acme", Area: "ops"},
		}},
	}}}
	s := NewServer(ledger.NewMemory(), "test").
		WithChannels(spy, nil).
		WithChannelListing(&listerSpy{kinds: []string{"slack"}})

	resp, err := s.ListChannels(as(domain.RoleCurator), openapi.ListChannelsRequestObject{})
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	page, ok := resp.(openapi.ListChannels200JSONResponse)
	if !ok {
		t.Fatalf("response = %T", resp)
	}
	got := page.Items[0].Conversations[0].Mode
	if got == nil || *got != "a-future-mode" {
		t.Fatalf("mode = %v, want the stored value", got)
	}
}

type channelSpy struct {
	listed       []admin.Channel
	bound        []admin.ChannelIdentity
	seen         []admin.ChannelAccountSeen
	putConv      admin.Conversation
	deletedScope domain.Scope
	putChannel   string
	deleted      string
	// governs is what the door decided about the caller's authority over the
	// installation. The administration is what acts on it, so what this end
	// has to keep true is that the answer travels.
	governs bool
	refuse  error
}

func (c *channelSpy) List(context.Context) ([]admin.Channel, error) { return c.listed, nil }

func (c *channelSpy) PutChannel(_ context.Context, w admin.ChannelWrite) error {
	c.putChannel, c.governs = w.Channel.Name, w.Governs
	if c.refuse != nil {
		return c.refuse
	}
	return nil
}

func (c *channelSpy) DeleteChannel(
	_ context.Context, name string, _ domain.UserID, governs bool,
) error {
	c.deleted, c.governs = name, governs
	if c.refuse != nil {
		return c.refuse
	}
	return nil
}

func (c *channelSpy) PutConversation(
	_ context.Context, _ string, conv admin.Conversation, _ domain.UserID,
) error {
	c.putConv = conv
	return c.refuse
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

func (c *channelSpy) SeenAccounts(context.Context) ([]admin.ChannelAccountSeen, error) {
	return c.seen, nil
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

func watchConversation(runAs string) openapi.PutConversationRequestObject {
	mode := openapi.Watch
	sources := []string{"B-alerts"}
	return openapi.PutConversationRequestObject{
		Name: "acme-slack", Conversation: "C-alerts",
		Body: &openapi.PutConversationJSONRequestBody{
			Company: "acme", Area: ptr("ops"),
			Mode: &mode, Sources: &sources,
			Agent: ptr("troubleshooting-sre"), RunAs: ptr(runAs),
		},
	}
}

func TestPutConversation_watchModeRunsAsTheConfigurerByDefault(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, err := s.PutConversation(as(domain.RoleCurator), watchConversation("usr_ana"))
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation204Response); !ok {
		t.Fatalf("response = %T, want accepted", resp)
	}
	if spy.putConv.RunAs != "usr_ana" {
		t.Fatalf("runAs = %q, want the configurer", spy.putConv.RunAs)
	}
}

func TestPutConversation_mentionsModeCanIncludeThreadContext(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)
	on := true
	mode := openapi.Mentions

	resp, err := s.PutConversation(as(domain.RoleCurator), openapi.PutConversationRequestObject{
		Name: "acme-slack", Conversation: "C-alerts",
		Body: &openapi.PutConversationJSONRequestBody{
			Company: "acme", Area: ptr("ops"),
			Mode: &mode, ThreadContext: &on,
		},
	})
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation204Response); !ok {
		t.Fatalf("response = %T, want accepted", resp)
	}
	if !spy.putConv.ThreadContext {
		t.Fatal("thread context was not stored for the mentions conversation")
	}
}

func TestPutConversation_bothModeKeepsMentionsAndWatchSettings(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)
	on := true
	mode := openapi.Both
	sources := []string{"B-alerts"}

	resp, err := s.PutConversation(as(domain.RoleCurator), openapi.PutConversationRequestObject{
		Name: "acme-slack", Conversation: "C-alerts",
		Body: &openapi.PutConversationJSONRequestBody{
			Company: "acme", Area: ptr("ops"),
			Mode: &mode, Sources: &sources,
			Agent: ptr("troubleshooting-sre"), RunAs: ptr("usr_ana"),
			ThreadContext: &on,
		},
	})
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation204Response); !ok {
		t.Fatalf("response = %T, want accepted", resp)
	}
	if spy.putConv.Mode != channel.ConversationBoth {
		t.Fatalf("mode = %q, want both", spy.putConv.Mode)
	}
	if !spy.putConv.ThreadContext {
		t.Fatal("thread context was not stored for the mention side")
	}
	if spy.putConv.Agent != "troubleshooting-sre" || spy.putConv.RunAs != "usr_ana" {
		t.Fatalf("watch settings = agent %q runAs %q, want kept",
			spy.putConv.Agent, spy.putConv.RunAs)
	}
}

func TestListChannels_saysWhichConversationsIncludeThreadContext(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{listed: []admin.Channel{{
		Name: "acme-slack", Kind: "slack",
		Conversations: []admin.Conversation{{
			ID: "C-alerts", Scope: domain.Scope{Company: "acme", Area: "ops"},
			ThreadContext: true,
		}},
	}}}
	s := NewServer(ledger.NewMemory(), "test").
		WithChannels(spy, nil).
		WithChannelListing(&listerSpy{})

	resp, err := s.ListChannels(as(domain.RoleCurator), openapi.ListChannelsRequestObject{})
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	page := resp.(openapi.ListChannels200JSONResponse)
	got := page.Items[0].Conversations[0].ThreadContext
	if got == nil || !*got {
		t.Fatalf("threadContext = %v, want true", got)
	}
}

func TestPutConversation_watchModeCannotDelegateRunAsWithoutIdentityAdministration(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, err := s.PutConversation(as(domain.RoleCurator), watchConversation("usr_bob"))
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want forbidden", resp)
	}
	if spy.putConv.RunAs != "" {
		t.Fatalf("conversation reached the store with runAs = %q", spy.putConv.RunAs)
	}
}

func TestPutConversation_installationAdministratorCanDelegateRunAs(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{}
	s := NewServer(ledger.NewMemory(), "test").
		WithChannels(spy, nil).
		WithPeople(&fakePeople{listed: []domain.Person{{
			ID: "usr_bob", Display: "Bob",
			Grants: []domain.HeldGrant{{
				Grant: domain.Grant{
					Scope: domain.Scope{Company: "acme", Area: "ops"},
					Role:  domain.RoleAuthor,
				},
			}},
		}}})

	resp, err := s.PutConversation(asInstallation(domain.RoleAdmin), watchConversation("usr_bob"))
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation204Response); !ok {
		t.Fatalf("response = %T, want accepted", resp)
	}
	if spy.putConv.RunAs != "usr_bob" {
		t.Fatalf("runAs = %q, want delegated principal", spy.putConv.RunAs)
	}
}

func TestPutConversation_watchModeRefusesRunAsWithoutAGrantInTheScope(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{}
	s := NewServer(ledger.NewMemory(), "test").
		WithChannels(spy, nil).
		WithPeople(&fakePeople{listed: []domain.Person{{
			ID: "usr_bob", Display: "Bob",
			Grants: []domain.HeldGrant{{
				Grant: domain.Grant{
					Scope: domain.Scope{Company: "other", Area: "ops"},
					Role:  domain.RoleAuthor,
				},
			}},
		}}})

	resp, err := s.PutConversation(asInstallation(domain.RoleAdmin), watchConversation("usr_bob"))
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want bad request", resp)
	}
	if spy.putConv.RunAs != "" {
		t.Fatalf("conversation reached the store with runAs = %q", spy.putConv.RunAs)
	}
}

func TestPutConversation_watchModeRefusesUnknownRunAsPrincipal(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{}
	s := NewServer(ledger.NewMemory(), "test").
		WithChannels(spy, nil).
		WithPeople(&fakePeople{listed: []domain.Person{{ID: "usr_bob", Display: "Bob"}}})

	resp, err := s.PutConversation(asInstallation(domain.RoleAdmin), watchConversation("usr_ghost"))
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want bad request", resp)
	}
	if spy.putConv.RunAs != "" {
		t.Fatalf("conversation reached the store with runAs = %q", spy.putConv.RunAs)
	}
}

func TestPutConversation_watchModeRequiresRunAs(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, err := s.PutConversation(as(domain.RoleCurator), watchConversation(""))
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want bad request", resp)
	}
	if spy.putConv.RunAs != "" {
		t.Fatalf("conversation reached the store with runAs = %q", spy.putConv.RunAs)
	}
}

func TestPutConversation_watchModeRefusesAnAgentWithoutConversationTrigger(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{}
	agents := &fakeAgents{published: []domain.AgentSummary{{
		ID:    "troubleshooting-sre",
		Scope: domain.Scope{Company: "acme", Area: "ops"},
	}}}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil).WithAgents(agents)

	resp, err := s.PutConversation(as(domain.RoleCurator), watchConversation("usr_ana"))
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want bad request", resp)
	}
	if spy.putConv.Agent != "" {
		t.Fatalf("conversation reached the store with agent = %q", spy.putConv.Agent)
	}
}

func TestPutConversation_watchModeAcceptsAnAgentWithConversationTrigger(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{}
	agents := &fakeAgents{published: []domain.AgentSummary{{
		ID:       "troubleshooting-sre",
		Scope:    domain.Scope{Company: "acme", Area: "ops"},
		Triggers: []domain.AgentTrigger{{Type: spec.TriggerChannel}},
	}}}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil).WithAgents(agents)

	resp, err := s.PutConversation(as(domain.RoleCurator), watchConversation("usr_ana"))
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation204Response); !ok {
		t.Fatalf("response = %T, want accepted", resp)
	}
	if spy.putConv.Agent != "troubleshooting-sre" {
		t.Fatalf("agent = %q, want the configured agent", spy.putConv.Agent)
	}
}

func TestListChannels_showsUnboundSeenAccountsAsBindingHints(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{
		listed: []admin.Channel{{Name: "acme-slack", Kind: "slack"}},
		bound: []admin.ChannelIdentity{
			{Channel: "acme-slack", Account: "U024", Principal: "usr_ana", Display: "Ana"},
		},
		seen: []admin.ChannelAccountSeen{
			{Channel: "acme-slack", Account: "U024"},
			{Channel: "acme-slack", Account: "U777", Conversation: "C-alerts"},
			{Channel: "other", Account: "U999"},
		},
	}
	s := NewServer(ledger.NewMemory(), "test").
		WithChannels(spy, nil).
		WithChannelListing(&listerSpy{})

	resp, err := s.ListChannels(as(domain.RoleCurator), openapi.ListChannelsRequestObject{})
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	page := resp.(openapi.ListChannels200JSONResponse)
	seen := page.Items[0].SeenAccounts
	if seen == nil || len(*seen) != 1 {
		t.Fatalf("seen = %+v, want one unbound account for this channel", seen)
	}
	if (*seen)[0].Account != "U777" || valueOr((*seen)[0].Conversation) != "C-alerts" {
		t.Fatalf("seen = %+v, want the unbound account with its last conversation", (*seen)[0])
	}
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

/*
The agent a conversation names is validated whatever starts its runs.

The check lived inside the watched-messages branch, so a conversation that only
takes mentions could be saved pointing at an agent that does not exist, lives
in another scope, or never declared the Conversation trigger. Nothing failed
until somebody mentioned the bot and read a refusal about a screen that had
said 204.
*/
func TestPutConversation_mentionsModeAgentOutsideTheScope_isRefused(t *testing.T) {
	t.Parallel()
	s := NewServer(ledger.NewMemory(), "test").
		WithChannels(&channelSpy{}, nil).
		WithAgents(&startableInScope{})

	resp, err := s.PutConversation(as(domain.RoleCurator), mentionsConversation("folha"))
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want the configuration refused", resp)
	}
}

func TestPutConversation_mentionsModeAgentWithNoConversationTrigger_isRefused(t *testing.T) {
	t.Parallel()
	s := NewServer(ledger.NewMemory(), "test").
		WithChannels(&channelSpy{}, nil).
		WithAgents(&startableInScope{})

	resp, err := s.PutConversation(as(domain.RoleCurator), mentionsConversation("internal-only"))
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want the configuration refused", resp)
	}
}

func TestPutConversation_mentionsModeStartableAgent_isStored(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{}
	s := NewServer(ledger.NewMemory(), "test").
		WithChannels(spy, nil).
		WithAgents(&startableInScope{})

	resp, err := s.PutConversation(as(domain.RoleCurator), mentionsConversation("triagem"))
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation204Response); !ok {
		t.Fatalf("response = %T, want accepted", resp)
	}
	if spy.putConv.Agent != "triagem" {
		t.Fatalf("agent = %q, want it stored", spy.putConv.Agent)
	}
}

// Nobody choosing is still a choice, and it must not be validated as if it
// named something.
func TestPutConversation_mentionsModeWithNoAgent_isStored(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{}
	s := NewServer(ledger.NewMemory(), "test").
		WithChannels(spy, nil).
		WithAgents(&startableInScope{})

	resp, err := s.PutConversation(as(domain.RoleCurator), mentionsConversation(""))
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation204Response); !ok {
		t.Fatalf("response = %T, want accepted", resp)
	}
	if spy.putConv.Agent != "" {
		t.Fatalf("agent = %q, want none", spy.putConv.Agent)
	}
}

func mentionsConversation(agent string) openapi.PutConversationRequestObject {
	mode := openapi.Mentions
	return openapi.PutConversationRequestObject{
		Name: "acme-slack", Conversation: "C-alerts",
		Body: &openapi.PutConversationJSONRequestBody{
			Company: "acme", Area: ptr("ops"),
			Mode: &mode, Agent: ptr(agent),
		},
	}
}

// startableInScope publishes one agent that a conversation may start and one
// that may not, in acme/ops and nowhere else.
type startableInScope struct{}

func (a *startableInScope) List(
	_ context.Context, scope domain.Scope, _ bool,
) ([]domain.AgentSummary, error) {
	if scope != (domain.Scope{Company: "acme", Area: "ops"}) {
		return nil, nil
	}
	return []domain.AgentSummary{
		{
			ID: "triagem", VersionID: "v1", Scope: scope,
			Triggers: []domain.AgentTrigger{{Type: spec.TriggerChannel}},
		},
		{ID: "internal-only", VersionID: "v1", Scope: scope},
	}, nil
}

func (a *startableInScope) Versions(
	context.Context, domain.AgentID,
) ([]domain.AgentSummary, error) {
	return nil, nil
}

func (a *startableInScope) Instructions(
	context.Context, domain.AgentID, domain.VersionID,
) (string, string, error) {
	return "", "", nil
}

/*
The choice survives the crossing, in both directions.

Every layer here is tested against objects built by hand, so each side can be
right while the wiring between them is not. Dropping the field from the request
leaves the console sending on and the server storing off; dropping it from the
response leaves the listing showing off, and the next edit saves that back over
somebody's choice. Neither shows up in a test of either side alone.
*/
func TestPutConversation_theDirectApprovalChoice_reachesTheStore(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, err := s.PutConversation(as(domain.RoleCurator), directApprovalConversation(true))
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation204Response); !ok {
		t.Fatalf("response = %T, want accepted", resp)
	}
	if !spy.putConv.DirectApprovals {
		t.Error("the choice did not reach the store")
	}
}

func TestListChannels_theDirectApprovalChoice_reachesTheConsole(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{listed: []admin.Channel{{
		Name: "acme-slack", Kind: "slack", Enabled: true,
		Conversations: []admin.Conversation{{
			ID: "C-alerts", Scope: domain.Scope{Company: "acme", Area: "ops"},
			Mode: "mentions", Wants: []string{"parked"},
			DirectApprovals: true, Enabled: true,
		}},
	}}}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, err := s.ListChannels(as(domain.RoleCurator), openapi.ListChannelsRequestObject{})
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	listed, ok := resp.(openapi.ListChannels200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the listing", resp)
	}
	conv := listed.Items[0].Conversations[0]
	if conv.DirectApprovals == nil || !*conv.DirectApprovals {
		t.Errorf("conversation = %+v, want the choice shown", conv)
	}
}

func directApprovalConversation(on bool) openapi.PutConversationRequestObject {
	mode := openapi.Mentions
	wants := []openapi.PutConversationJSONBodyWants{
		openapi.PutConversationJSONBodyWantsParked,
	}
	return openapi.PutConversationRequestObject{
		Name: "acme-slack", Conversation: "C-alerts",
		Body: &openapi.PutConversationJSONRequestBody{
			Company: "acme", Area: ptr("ops"),
			Mode: &mode, Wants: &wants, DirectApprovals: &on,
		},
	}
}

/*
A room for the whole installation needs authority over the installation.

Every other conversation is configured inside a scope somebody already governs.
This one reads every company's runs into one place, which is the disclosure the
conversation scope exists to prevent, arriving as a notification.

Configuring channels is a curator's act, and a curator granted on one company
holds it — so without this a company-level configurer could build a room
receiving another company's runs. Deciding that one room hears the whole
installation is the authority above them all.
*/
func TestPutConversation_forTheWholeInstallation_needsAuthorityOverIt(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, err := s.PutConversation(as(domain.RoleCurator), installationConversationRequest())
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want it refused", resp)
	}
	if spy.putConv.ID != "" {
		t.Error("the conversation reached the store")
	}
}

func TestPutConversation_forTheWholeInstallation_isAllowedToWhoGovernsIt(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, err := s.PutConversation(asInstallation(domain.RoleAdmin), installationConversationRequest())
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation204Response); !ok {
		t.Fatalf("response = %T, want accepted", resp)
	}
}

// And removing it needs the same. Otherwise a company configurer silences the
// one room that hears what nobody else does.
func TestDeleteConversation_forTheWholeInstallation_needsAuthorityOverIt(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{listed: []admin.Channel{{
		Name: "acme-slack", Kind: "slack", Enabled: true,
		Conversations: []admin.Conversation{{
			ID: "C-everywhere", Scope: domain.Scope{Company: domain.Installation},
			Mode: "announce", Enabled: true,
		}},
	}}}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, err := s.DeleteConversation(as(domain.RoleCurator),
		openapi.DeleteConversationRequestObject{Name: "acme-slack", Conversation: "C-everywhere"})
	if err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	if _, ok := resp.(openapi.DeleteConversation403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want it refused", resp)
	}
	if spy.deletedScope != (domain.Scope{}) {
		t.Error("the removal reached the store")
	}
}

// An installation conversation names no agent, so the check that would look for
// one has nothing to look in. Told the truth rather than sent to publish an
// agent into a scope nothing can be published to.
func TestPutConversation_forTheWholeInstallation_doesNotAskForAnAgent(t *testing.T) {
	t.Parallel()
	s := NewServer(ledger.NewMemory(), "test").
		WithChannels(&channelSpy{}, nil).
		WithAgents(&startableInScope{})

	req := installationConversationRequest()
	req.Body.Agent = ptr("triagem")

	resp, err := s.PutConversation(asInstallation(domain.RoleAdmin), req)
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	if _, ok := resp.(openapi.PutConversation204Response); !ok {
		t.Fatalf("response = %T, want accepted with the agent ignored", resp)
	}
}

func installationConversationRequest() openapi.PutConversationRequestObject {
	wants := []openapi.PutConversationJSONBodyWants{
		openapi.PutConversationJSONBodyWantsParked,
	}
	return openapi.PutConversationRequestObject{
		Name: "acme-slack", Conversation: "C-everywhere",
		Body: &openapi.PutConversationJSONRequestBody{
			Company: string(domain.Installation), Wants: &wants,
		},
	}
}
