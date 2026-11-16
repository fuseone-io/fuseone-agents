package httpapi

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/settings"
)

/*
A connection is written at the scope its request names, by whoever holds
provider:write there or at the platform's administration (NT-019, phase 4
writers): the same rule as every other kind.
*/

var cxScope = domain.Scope{Company: "acme", Area: "cx"}

func asAreaAdmin(area domain.Scope) context.Context {
	return auth.WithPrincipal(context.Background(), domain.Principal{
		ID: "usr_cx", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: area, Role: domain.RoleAdmin}},
	})
}

func putChannelAt(kind openapi.ConnectorScopeKind, company, area string) openapi.PutChannelRequestObject {
	return openapi.PutChannelRequestObject{
		Name: "cx-slack", Body: &openapi.PutChannelJSONRequestBody{
			Kind: "slack", ScopeKind: &kind, Company: &company, Area: &area,
		},
	}
}

func TestPutChannel_anAreasAdministrator_writesTheirAreaAndNothingElse(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		req    openapi.PutChannelRequestObject
		stored bool
	}{
		"own area":     {putChannelAt("area", "acme", "cx"), true},
		"sibling area": {putChannelAt("area", "acme", "finance"), false},
		"the company":  {putChannelAt("company", "acme", ""), false},
		"installation": {openapi.PutChannelRequestObject{Name: "cx-slack", Body: &openapi.PutChannelJSONRequestBody{Kind: "slack"}}, false},
	} {
		t.Run(name, func(t *testing.T) {
			spy := &channelSpy{}
			s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)
			resp, err := s.PutChannel(asAreaAdmin(cxScope), c.req)
			if err != nil {
				t.Fatalf("PutChannel: %v", err)
			}
			_, accepted := resp.(openapi.PutChannel204Response)
			if accepted != c.stored {
				t.Fatalf("response = %T, want stored=%v", resp, c.stored)
			}
			if c.stored && (spy.wrote.At != settings.ScopeArea || spy.wrote.Stored != cxScope) {
				t.Errorf("wrote at %s %+v, want acme/cx", spy.wrote.At, spy.wrote.Stored)
			}
			if !c.stored && spy.putChannel != "" {
				t.Errorf("a refused write reached the administration")
			}
		})
	}
}

// The platform's administration governs every area, so it writes acme/cx's
// connection too.
func TestPutChannel_thePlatformsAdministrator_writesAnArea(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, err := s.PutChannel(as(domain.RoleAdmin), putChannelAt("area", "acme", "cx"))
	if err != nil {
		t.Fatalf("PutChannel: %v", err)
	}
	if _, ok := resp.(openapi.PutChannel204Response); !ok || spy.wrote.Stored != cxScope {
		t.Fatalf("response = %T at %+v, want acme/cx written", resp, spy.wrote.Stored)
	}
}

func TestPutChannel_aNameAnotherScopeHolds_is409(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{refuse: settings.ErrNameTaken}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, err := s.PutChannel(asAreaAdmin(cxScope), putChannelAt("area", "acme", "cx"))
	if err != nil {
		t.Fatalf("PutChannel: %v", err)
	}
	if _, ok := resp.(openapi.PutChannel409ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want 409", resp)
	}
}

// A removal is authorized where the connection is stored, never where the
// caller says: acme/cx's administrator removes acme/cx's connection, and not
// the installation's.
func TestDeleteChannel_isAuthorizedWhereTheConnectionIsStored(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{listed: []admin.Channel{
		{Name: "cx-slack", At: settings.ScopeArea, Stored: cxScope},
		{Name: "acme-slack", At: settings.ScopeInstallation},
	}}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, err := s.DeleteChannel(asAreaAdmin(cxScope), openapi.DeleteChannelRequestObject{Name: "cx-slack"})
	if err != nil {
		t.Fatalf("DeleteChannel: %v", err)
	}
	if _, ok := resp.(openapi.DeleteChannel204Response); !ok || spy.deleted != "cx-slack" {
		t.Fatalf("response = %T, want acme/cx's connection removed", resp)
	}

	spy.deleted = ""
	resp, err = s.DeleteChannel(asAreaAdmin(cxScope), openapi.DeleteChannelRequestObject{Name: "acme-slack"})
	if err != nil {
		t.Fatalf("DeleteChannel: %v", err)
	}
	if _, ok := resp.(openapi.DeleteChannel403ApplicationProblemPlusJSONResponse); !ok || spy.deleted != "" {
		t.Fatalf("response = %T, want the installation's connection refused", resp)
	}
}

// Read back where it is stored: acme/cx's administrator reads acme/cx's
// connection and no sibling's.
func TestGetChannel_isAuthorizedWhereTheConnectionIsStored(t *testing.T) {
	t.Parallel()
	spy := &channelSpy{listed: []admin.Channel{
		{Name: "cx-slack", At: settings.ScopeArea, Stored: cxScope},
		{Name: "fin-slack", At: settings.ScopeArea, Stored: domain.Scope{Company: "acme", Area: "finance"}},
	}}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, _ := s.GetChannel(asAreaAdmin(cxScope), openapi.GetChannelRequestObject{Name: "cx-slack"})
	if _, ok := resp.(openapi.GetChannel200JSONResponse); !ok {
		t.Fatalf("own = %T, want it read", resp)
	}
	resp, _ = s.GetChannel(asAreaAdmin(cxScope), openapi.GetChannelRequestObject{Name: "fin-slack"})
	if _, ok := resp.(openapi.GetChannel403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("sibling = %T, want it refused", resp)
	}
}

/*
The listing shows a configurer their own connections, editable, and the ones
they inherit, read-only, never a sibling area's. An inherited connection
carries only the conversations the caller may see, and no bindings or seen
accounts: those are other people's.
*/
func TestListChannels_anAreasAdministrator_seesTheirsAndWhatTheyInherit(t *testing.T) {
	t.Parallel()
	finance := domain.Scope{Company: "acme", Area: "finance"}
	spy := &channelSpy{
		listed: []admin.Channel{
			{Name: "cx-slack", At: settings.ScopeArea, Stored: cxScope},
			{Name: "fin-slack", At: settings.ScopeArea, Stored: finance},
			{Name: "acme-slack", At: settings.ScopeInstallation, Conversations: []admin.Conversation{
				{ID: "C-cx", Scope: cxScope}, {ID: "C-fin", Scope: finance},
			}},
		},
		bound: []admin.ChannelIdentity{{Channel: "acme-slack", Account: "U1", Principal: "usr_bia"}},
		seen:  []admin.ChannelAccountSeen{{Channel: "acme-slack", Account: "U2"}},
	}
	s := NewServer(ledger.NewMemory(), "test").WithChannels(spy, nil)

	resp, err := s.ListChannels(asAreaAdmin(cxScope), openapi.ListChannelsRequestObject{})
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	listed, ok := resp.(openapi.ListChannels200JSONResponse)
	if !ok {
		t.Fatalf("response = %T", resp)
	}
	got := map[string]openapi.Channel{}
	for _, one := range listed.Items {
		got[one.Name] = one
	}
	if _, sibling := got["fin-slack"]; sibling {
		t.Error("a sibling area's connection is listed")
	}
	if own := got["cx-slack"]; own.Editable == nil || !*own.Editable {
		t.Errorf("own = %+v, want it editable", own)
	}
	inherited, ok := got["acme-slack"]
	if !ok || inherited.Editable == nil || *inherited.Editable {
		t.Fatalf("inherited = %+v, want it listed read-only", inherited)
	}
	if len(inherited.Conversations) != 1 || inherited.Conversations[0].Id != "C-cx" {
		t.Errorf("inherited conversations = %+v, want only acme/cx's", inherited.Conversations)
	}
	if inherited.Identities != nil && len(*inherited.Identities) > 0 {
		t.Errorf("inherited identities = %+v, want none", *inherited.Identities)
	}
	if inherited.SeenAccounts != nil && len(*inherited.SeenAccounts) > 0 {
		t.Errorf("inherited seen accounts = %+v, want none", *inherited.SeenAccounts)
	}
}
