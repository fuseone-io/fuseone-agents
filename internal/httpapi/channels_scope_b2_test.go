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
	"github.com/fuseone/agents/internal/settings"
)

/*
A conversation's doors are at the conversation's scope, and a binding's at its
connection's (NT-019, phase 4 writers). On an area's connection, only a person
holding a grant inside the area may be bound.
*/

var financeScope = domain.Scope{Company: "acme", Area: "finance"}

func scopedChannelServer(spy *channelSpy, people ...domain.Person) *Server {
	return NewServer(ledger.NewMemory(), "test").WithChannels(spy, &postedTo{}).
		WithPeople(&fakePeople{listed: people})
}

// postedTo is an announcer that records where it posted. rowAt is where the
// connection's row stands when the post reads it, the installation's when
// zero, which is what its allow is judged on.
type postedTo struct {
	to    []string
	rowAt domain.Scope
}

func (p *postedTo) PostIf(
	_ context.Context, c channel.Conversation, _ channel.Message, allow func(settings.ScopeKind, domain.Scope) error,
) (string, error) {
	kind, at := settings.ScopeInstallation, domain.Scope{}
	if p.rowAt != (domain.Scope{}) {
		kind, at = settings.ScopeArea, p.rowAt
	}
	if allow != nil {
		if err := allow(kind, at); err != nil {
			return "", err
		}
	}
	p.to = append(p.to, c.ID)
	return "ref", nil
}

func conversationAt(company, area string) openapi.PutConversationRequestObject {
	return openapi.PutConversationRequestObject{
		Name: "acme-slack", Conversation: "C1",
		Body: &openapi.PutConversationJSONRequestBody{Company: company, Area: &area},
	}
}

func TestPutConversation_anAreasAdministrator_configuresTheirAreasOnly(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		req    openapi.PutConversationRequestObject
		stored bool
	}{
		"own area":     {conversationAt("acme", "cx"), true},
		"sibling area": {conversationAt("acme", "finance"), false},
		"the company":  {conversationAt("acme", ""), false},
		"installation": {conversationAt(string(domain.Installation), ""), false},
	} {
		t.Run(name, func(t *testing.T) {
			spy := &channelSpy{}
			resp, err := scopedChannelServer(spy).PutConversation(asAreaAdmin(cxScope), c.req)
			if err != nil {
				t.Fatalf("PutConversation: %v", err)
			}
			_, accepted := resp.(openapi.PutConversation204Response)
			if accepted != c.stored || (!c.stored && spy.putConv.ID != "") {
				t.Fatalf("response = %T, stored %q; want stored=%v", resp, spy.putConv.ID, c.stored)
			}
		})
	}
}

// Removed and tested where the conversation is stored: acme/cx's
// administrator reaches acme/cx's conversation and not acme/finance's.
func TestConversationDoors_areAuthorizedWhereTheConversationIsStored(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{listed: []admin.Channel{{Name: "acme-slack", Conversations: []admin.Conversation{
		{ID: "C-cx", Scope: cxScope}, {ID: "C-fin", Scope: financeScope},
	}}}}
	s := scopedChannelServer(spy)
	ctx := asAreaAdmin(cxScope)

	if resp, _ := s.DeleteConversation(ctx, openapi.DeleteConversationRequestObject{Name: "acme-slack", Conversation: "C-cx"}); !is[openapi.DeleteConversation204Response](resp) {
		t.Errorf("delete own = %T, want removed", resp)
	}
	if resp, _ := s.DeleteConversation(ctx, openapi.DeleteConversationRequestObject{Name: "acme-slack", Conversation: "C-fin"}); !is[openapi.DeleteConversation403ApplicationProblemPlusJSONResponse](resp) {
		t.Errorf("delete sibling's = %T, want refused", resp)
	}
	if resp, _ := s.TestConversation(ctx, openapi.TestConversationRequestObject{Name: "acme-slack", Conversation: "C-fin"}); !is[openapi.TestConversation403ApplicationProblemPlusJSONResponse](resp) {
		t.Errorf("test sibling's = %T, want refused", resp)
	}
	if resp, _ := s.TestConversation(ctx, openapi.TestConversationRequestObject{Name: "acme-slack", Conversation: "C-cx"}); !is[openapi.TestConversation200JSONResponse](resp) {
		t.Errorf("test own = %T, want posted", resp)
	}
}

/*
On acme/cx's own connection, acme/cx's administrator binds a person holding a
grant inside acme/cx and nobody else: a grant over the area, at the company, is
not one inside it. On the platform's connection they bind nobody.
*/
func TestBindChannelIdentity_onAnAreasConnection_onlyPeopleInsideTheArea(t *testing.T) {
	t.Parallel()
	person := func(id string, at domain.Scope) domain.Person {
		return domain.Person{ID: id, Grants: []domain.HeldGrant{{Grant: domain.Grant{Scope: at, Role: domain.RoleApprover}}}}
	}
	spy := &channelSpy{listed: []admin.Channel{
		{Name: "cx-slack", At: settings.ScopeArea, Stored: cxScope},
		{Name: "acme-slack", At: settings.ScopeInstallation},
	}}
	s := scopedChannelServer(spy,
		person("usr_in", cxScope), person("usr_company", domain.Scope{Company: "acme"}),
		person("usr_fin", financeScope))
	ctx := asAreaAdmin(cxScope)
	bind := func(conn, who string) openapi.BindChannelIdentityResponseObject {
		resp, err := s.BindChannelIdentity(ctx, openapi.BindChannelIdentityRequestObject{
			Name: conn, Account: "U-" + who, Body: &openapi.BindChannelIdentityJSONRequestBody{Principal: who},
		})
		if err != nil {
			t.Fatalf("BindChannelIdentity: %v", err)
		}
		return resp
	}

	if resp := bind("cx-slack", "usr_in"); !is[openapi.BindChannelIdentity204Response](resp) {
		t.Errorf("a person inside acme/cx = %T, want bound", resp)
	}
	for _, who := range []string{"usr_company", "usr_fin", "usr_nobody"} {
		if resp := bind("cx-slack", who); !is[openapi.BindChannelIdentity400ApplicationProblemPlusJSONResponse](resp) {
			t.Errorf("%s = %T, want refused", who, resp)
		}
	}
	if resp := bind("acme-slack", "usr_in"); !is[openapi.BindChannelIdentity403ApplicationProblemPlusJSONResponse](resp) {
		t.Errorf("the platform's connection = %T, want refused", resp)
	}
	if len(spy.bound) != 1 || spy.bound[0].Principal != "usr_in" {
		t.Errorf("bound = %+v, want only the person inside acme/cx", spy.bound)
	}
}

func TestUnbindChannelIdentity_isAuthorizedWhereTheConnectionIsStored(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{listed: []admin.Channel{
		{Name: "cx-slack", At: settings.ScopeArea, Stored: cxScope},
		{Name: "acme-slack", At: settings.ScopeInstallation},
	}}
	s := scopedChannelServer(spy)
	ctx := asAreaAdmin(cxScope)

	if resp, _ := s.UnbindChannelIdentity(ctx, openapi.UnbindChannelIdentityRequestObject{Name: "cx-slack", Account: "U1"}); !is[openapi.UnbindChannelIdentity204Response](resp) || spy.unbound != "cx-slack" {
		t.Errorf("own = %T, want unbound", resp)
	}
	spy.unbound = ""
	if resp, _ := s.UnbindChannelIdentity(ctx, openapi.UnbindChannelIdentityRequestObject{Name: "acme-slack", Account: "U1"}); !is[openapi.UnbindChannelIdentity403ApplicationProblemPlusJSONResponse](resp) || spy.unbound != "" {
		t.Errorf("the platform's = %T, want refused", resp)
	}
}

func is[T any](v any) bool { _, ok := v.(T); return ok }

// The picker lists the rooms a connection's bot is in, which takes the
// connection's token: offered to whoever configures the connection, and to
// nobody else.
func TestListAvailableConversations_isAuthorizedWhereTheConnectionIsStored(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{listed: []admin.Channel{
		{Name: "cx-slack", At: settings.ScopeArea, Stored: cxScope},
		{Name: "fin-slack", At: settings.ScopeArea, Stored: financeScope},
	}}
	s := scopedChannelServer(spy).WithChannelListing(roomsOf{rowAt: map[string]domain.Scope{
		"cx-slack": cxScope, "fin-slack": financeScope,
	}})
	ctx := asAreaAdmin(cxScope)

	if resp, _ := s.ListAvailableConversations(ctx, openapi.ListAvailableConversationsRequestObject{Name: "cx-slack"}); !is[openapi.ListAvailableConversations200JSONResponse](resp) {
		t.Errorf("own = %T, want listed", resp)
	}
	if resp, _ := s.ListAvailableConversations(ctx, openapi.ListAvailableConversationsRequestObject{Name: "fin-slack"}); !is[openapi.ListAvailableConversations403ApplicationProblemPlusJSONResponse](resp) {
		t.Errorf("sibling's = %T, want refused", resp)
	}
}

// roomsOf lists rooms through a connection whose row stands at rowAt when
// the listing reads it: where its allow is judged. vendorRefuses is the
// vendor's own refusal, after the connection was judged.
type roomsOf struct {
	rowAt         map[string]domain.Scope
	vendorRefuses error
}

func (r roomsOf) ConversationsIf(
	_ context.Context, name string, allow func(settings.ScopeKind, domain.Scope) error,
) ([]channel.Available, error) {
	at, ok := r.rowAt[name]
	if !ok {
		return nil, errors.New("no such connection")
	}
	if allow != nil {
		if err := allow(settings.ScopeArea, at); err != nil {
			return nil, err
		}
	}
	if r.vendorRefuses != nil {
		return nil, r.vendorRefuses
	}
	return []channel.Available{{ID: "C1", Name: "#ops"}}, nil
}
func (roomsOf) Kinds() []string { return []string{"slack"} }

/*
What is judged is the connection that is used (NT-019, phase 4 writers). The
listing said cx-slack was acme/cx's; by the time the picker and the test post
read it to use its token, it had been replaced by acme/finance's. Neither
speaks through finance's app.
*/
func TestConnectionUse_isJudgedOnTheRowItUses(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{listed: []admin.Channel{{Name: "cx-slack", At: settings.ScopeArea, Stored: cxScope,
		Conversations: []admin.Conversation{{ID: "C-cx", Scope: cxScope}}}}}
	announcer := &postedTo{rowAt: financeScope}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, announcer).
		WithChannelListing(roomsOf{rowAt: map[string]domain.Scope{"cx-slack": financeScope}})
	ctx := asAreaAdmin(cxScope)

	if resp, _ := s.ListAvailableConversations(ctx, openapi.ListAvailableConversationsRequestObject{Name: "cx-slack"}); is[openapi.ListAvailableConversations200JSONResponse](resp) {
		t.Errorf("picker = %T, want finance's app not listed for acme/cx", resp)
	}
	if resp, _ := s.TestConversation(ctx, openapi.TestConversationRequestObject{Name: "cx-slack", Conversation: "C-cx"}); is[openapi.TestConversation200JSONResponse](resp) {
		t.Errorf("test post = %T, want acme/cx's room not posted through finance's app", resp)
	}
	if len(announcer.to) != 0 {
		t.Errorf("posted to %v, want nothing", announcer.to)
	}
}

// A room for the whole installation is tested by whoever governs it, as it is
// written and removed: a curator of the platform's administration, who holds
// provider:write there and governs nothing, posts nothing into it.
func TestTestConversation_aRoomForTheInstallation_needsAuthorityOverIt(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{listed: []admin.Channel{{Name: "acme-slack", Conversations: []admin.Conversation{
		{ID: "C-all", Scope: domain.Scope{Company: domain.Installation}},
	}}}}
	announcer := &postedTo{}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, announcer)

	resp, _ := s.TestConversation(as(domain.RoleCurator), openapi.TestConversationRequestObject{Name: "acme-slack", Conversation: "C-all"})
	if !is[openapi.TestConversation403ApplicationProblemPlusJSONResponse](resp) || len(announcer.to) != 0 {
		t.Fatalf("response = %T, posted %v; want refused with nothing posted", resp, announcer.to)
	}
}

// The vendor's refusal on the caller's own connection is theirs to read: an
// app that can post and cannot list says so, and the picker passes it on.
func TestListAvailableConversations_theVendorsRefusalOnTheirOwnConnection_isTheirs(t *testing.T) {
	t.Parallel()
	s := scopedChannelServer(&channelSpy{}).WithChannelListing(roomsOf{
		rowAt: map[string]domain.Scope{"cx-slack": cxScope}, vendorRefuses: errors.New("missing_scope: channels:read"),
	})

	resp, _ := s.ListAvailableConversations(asAreaAdmin(cxScope), openapi.ListAvailableConversationsRequestObject{Name: "cx-slack"})
	if !is[openapi.ListAvailableConversations400ApplicationProblemPlusJSONResponse](resp) {
		t.Fatalf("response = %T, want the vendor's reason", resp)
	}
	resp, _ = s.ListAvailableConversations(asAreaAdmin(cxScope), openapi.ListAvailableConversationsRequestObject{Name: "nowhere-slack"})
	if !is[openapi.ListAvailableConversations403ApplicationProblemPlusJSONResponse](resp) {
		t.Fatalf("an unknown name = %T, want refused as the installation's", resp)
	}
}
