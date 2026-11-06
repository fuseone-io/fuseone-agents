package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
Reading one resource by name, from the door.

Who may read, and what the answer is when they may not, is held here; what the
read carries against what was written is held in cmd/agentd, against real
stores. A caller without the authority learns nothing from the name they asked
about: the refusal comes before the lookup, so it cannot say whether anything
is there.
*/

// refusedCallers holds tool:read or a grant elsewhere, and not configure in the
// administrative scope.
func refusedCallers() map[string]context.Context {
	return map[string]context.Context{
		"an author, who can read the listing": as(domain.RoleAuthor),
		"an auditor, who reads everything":    as(domain.RoleAuditor),
		"a curator of another area":           inArea("cx", domain.RoleCurator),
	}
}

func TestGetMCPServer_withoutTheAuthorityToConfigure_refusesTheSameWhetherItExistsOrNot(t *testing.T) {
	t.Parallel()
	s := serverWith(t, &fakeAdmin{servers: []domain.MCPServer{{Name: "github", Enabled: true}}})
	for name, caller := range refusedCallers() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			there, err := s.GetMCPServer(caller, openapi.GetMCPServerRequestObject{Name: "github"})
			if err != nil {
				t.Fatalf("GetMCPServer: %v", err)
			}
			absent, err := s.GetMCPServer(caller, openapi.GetMCPServerRequestObject{Name: "nothing"})
			if err != nil {
				t.Fatalf("GetMCPServer: %v", err)
			}
			if _, refused := there.(openapi.GetMCPServer403ApplicationProblemPlusJSONResponse); !refused {
				t.Fatalf("existing = %T, want 403", there)
			}
			if !reflect.DeepEqual(there, absent) {
				t.Errorf("the refusal says whether it exists\n there: %#v\nabsent: %#v", there, absent)
			}
		})
	}
}

func TestGetMCPServer_nameNobodyConfigured_isNotFound(t *testing.T) {
	t.Parallel()
	s := serverWith(t, &fakeAdmin{servers: []domain.MCPServer{{Name: "github", Enabled: true}}})
	resp, err := s.GetMCPServer(as(domain.RoleCurator), openapi.GetMCPServerRequestObject{Name: "gith"})
	if err != nil {
		t.Fatalf("GetMCPServer: %v", err)
	}
	if _, absent := resp.(openapi.GetMCPServer404ApplicationProblemPlusJSONResponse); !absent {
		t.Fatalf("response = %T, want 404", resp)
	}
}

// A server a worker reports and nobody configured has nothing PUT could have
// written, so a client reading it would see drift it can never resolve.
func TestGetMCPServer_observedAndNotConfigured_isNotFound(t *testing.T) {
	t.Parallel()
	s := serverWith(t, &fakeAdmin{}).WithHealth(fakeHealth{
		"from-a-flag": {Reachable: true, ObservedAt: time.Now()},
	})
	resp, err := s.GetMCPServer(as(domain.RoleCurator), openapi.GetMCPServerRequestObject{Name: "from-a-flag"})
	if err != nil {
		t.Fatalf("GetMCPServer: %v", err)
	}
	if _, absent := resp.(openapi.GetMCPServer404ApplicationProblemPlusJSONResponse); !absent {
		t.Fatalf("response = %T, want 404", resp)
	}
}

func TestGetMCPServer_asAConfigurer_answersConfigurationAndNothingObserved(t *testing.T) {
	t.Parallel()
	surface := []string{"search"}
	s := serverWith(t, &fakeAdmin{servers: []domain.MCPServer{{
		Name: "github", Transport: domain.TransportHTTP, URL: "https://mcp.example.test",
		Surface: &surface, HasSecret: true, Enabled: true, UpdatedBy: "usr_ana",
	}}}).WithHealth(fakeHealth{
		"github": {Reachable: true, ObservedAt: time.Now()},
	})

	resp, err := s.GetMCPServer(as(domain.RoleCurator), openapi.GetMCPServerRequestObject{Name: "github"})
	if err != nil {
		t.Fatalf("GetMCPServer: %v", err)
	}
	got, ok := resp.(openapi.GetMCPServer200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want 200", resp)
	}
	if got.Name != "github" || valueOr(got.Url) != "https://mcp.example.test" ||
		!valueOr(got.HasSecret) || got.Surface == nil || len(*got.Surface) != 1 {
		t.Errorf("configuration = %+v", got)
	}
	// What a worker saw and who is asking both vary with nobody having written
	// anything, and a client diffing this body would read them as drift.
	if got.Health != nil || got.CallAuth != nil || got.Egress != nil {
		t.Errorf("the read carries what was observed: %+v", got)
	}
}

// The handler has the sealed half in reach, through the same store, and a
// response is only safe if nothing in it came from there.
func TestGetMCPServer_storeHoldsACredential_noResponseFieldCarriesIt(t *testing.T) {
	t.Parallel()
	const secret = "stored-bearer-secret"
	s := serverWith(t, &fakeAdmin{
		token:   secret,
		servers: []domain.MCPServer{{Name: "github", Transport: domain.TransportHTTP, HasSecret: true, Enabled: true}},
	})
	resp, err := s.GetMCPServer(as(domain.RoleCurator), openapi.GetMCPServerRequestObject{Name: "github"})
	if err != nil {
		t.Fatalf("GetMCPServer: %v", err)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.Contains(string(raw), `"hasSecret":true`) {
		t.Errorf("the read does not say a credential is stored: %s", raw)
	}
	if strings.Contains(string(raw), secret) {
		t.Errorf("the read carries the credential: %s", raw)
	}
}

func TestGetModelProvider_withoutTheAuthorityToConfigure_refusesTheSameWhetherItExistsOrNot(t *testing.T) {
	t.Parallel()
	s := serverWith(t, &fakeAdmin{providers: []domain.ModelProvider{{Name: "gateway", Enabled: true}}})
	for name, caller := range refusedCallers() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			there, err := s.GetModelProvider(caller, openapi.GetModelProviderRequestObject{Name: "gateway"})
			if err != nil {
				t.Fatalf("GetModelProvider: %v", err)
			}
			absent, err := s.GetModelProvider(caller, openapi.GetModelProviderRequestObject{Name: "nothing"})
			if err != nil {
				t.Fatalf("GetModelProvider: %v", err)
			}
			if _, refused := there.(openapi.GetModelProvider403ApplicationProblemPlusJSONResponse); !refused {
				t.Fatalf("existing = %T, want 403", there)
			}
			if !reflect.DeepEqual(there, absent) {
				t.Errorf("the refusal says whether it exists\n there: %#v\nabsent: %#v", there, absent)
			}
		})
	}
}

func TestGetModelProvider_asAConfigurer_answersWhatIsStoredOrNotFound(t *testing.T) {
	t.Parallel()
	s := serverWith(t, &fakeAdmin{providers: []domain.ModelProvider{{
		Name: "gateway", Kind: "openai_compatible", BaseURL: "https://models.example.test",
		Models: []string{"m1"}, Enabled: true, HasKey: true,
	}}})

	resp, err := s.GetModelProvider(as(domain.RoleCurator), openapi.GetModelProviderRequestObject{Name: "gateway"})
	if err != nil {
		t.Fatalf("GetModelProvider: %v", err)
	}
	got, ok := resp.(openapi.GetModelProvider200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want 200", resp)
	}
	if got.Name != "gateway" || got.BaseUrl != "https://models.example.test" || !got.HasKey ||
		got.Models == nil || len(*got.Models) != 1 {
		t.Errorf("provider = %+v", got)
	}

	resp, err = s.GetModelProvider(as(domain.RoleCurator), openapi.GetModelProviderRequestObject{Name: "gate"})
	if err != nil {
		t.Fatalf("GetModelProvider: %v", err)
	}
	if _, absent := resp.(openapi.GetModelProvider404ApplicationProblemPlusJSONResponse); !absent {
		t.Fatalf("response = %T, want 404", resp)
	}
}

func TestGetModelProvider_noAdministrationStore_failsLikeItsNeighbour(t *testing.T) {
	t.Parallel()
	s := NewServer(ledger.NewMemory(), "test")
	_, err := s.GetModelProvider(as(domain.RoleCurator), openapi.GetModelProviderRequestObject{Name: "gateway"})
	if !errors.Is(err, errNoAdministration) {
		t.Fatalf("err = %v, want errNoAdministration", err)
	}
}

func TestGetChannel_withoutTheAuthorityToConfigure_refusesTheSameWhetherItExistsOrNot(t *testing.T) {
	t.Parallel()
	s := NewServer(ledger.NewMemory(), "test").WithChannels(&channelSpy{
		listed: []admin.Channel{{Name: "acme-slack", Kind: "slack", Enabled: true}},
	}, nil)
	for name, caller := range refusedCallers() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			there, err := s.GetChannel(caller, openapi.GetChannelRequestObject{Name: "acme-slack"})
			if err != nil {
				t.Fatalf("GetChannel: %v", err)
			}
			absent, err := s.GetChannel(caller, openapi.GetChannelRequestObject{Name: "nothing"})
			if err != nil {
				t.Fatalf("GetChannel: %v", err)
			}
			if _, refused := there.(openapi.GetChannel403ApplicationProblemPlusJSONResponse); !refused {
				t.Fatalf("existing = %T, want 403", there)
			}
			if !reflect.DeepEqual(there, absent) {
				t.Errorf("the refusal says whether it exists\n there: %#v\nabsent: %#v", there, absent)
			}
		})
	}
}

func TestGetChannel_asAConfigurer_answersConfigurationAndNoAccountsSeen(t *testing.T) {
	t.Parallel()
	s := NewServer(ledger.NewMemory(), "test").WithChannels(&channelSpy{
		listed: []admin.Channel{{
			Name: "acme-slack", Kind: "slack", Workspace: "Acme", Enabled: true, HasCredential: true,
			Conversations: []admin.Conversation{{ID: "C1", Enabled: true}},
		}, {Name: "other", Kind: "slack", Enabled: true}},
		bound: []admin.ChannelIdentity{
			{Channel: "acme-slack", Account: "U1", Principal: "usr_ana"},
			{Channel: "other", Account: "U2", Principal: "usr_bia"},
		},
		seen: []admin.ChannelAccountSeen{{Channel: "acme-slack", Account: "U3"}},
	}, nil)

	resp, err := s.GetChannel(as(domain.RoleCurator), openapi.GetChannelRequestObject{Name: "acme-slack"})
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	got, ok := resp.(openapi.GetChannel200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want 200", resp)
	}
	if got.Name != "acme-slack" || valueOr(got.Workspace) != "Acme" || !got.HasCredential ||
		len(got.Conversations) != 1 {
		t.Errorf("channel = %+v", got)
	}
	if got.Identities == nil || len(*got.Identities) != 1 || (*got.Identities)[0].Account != "U1" {
		t.Errorf("identities = %+v, want this connection's own", got.Identities)
	}
	// Who interacted changes with nobody having written anything, and an empty
	// list here would say nobody did.
	if got.SeenAccounts != nil {
		t.Errorf("seenAccounts = %+v, want it left out", *got.SeenAccounts)
	}
}

func TestGetChannel_nameNobodyConfigured_isNotFound(t *testing.T) {
	t.Parallel()
	s := NewServer(ledger.NewMemory(), "test").WithChannels(&channelSpy{
		listed: []admin.Channel{{Name: "acme-slack", Kind: "slack", Enabled: true}},
	}, nil)
	resp, err := s.GetChannel(as(domain.RoleCurator), openapi.GetChannelRequestObject{Name: "acme"})
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	if _, absent := resp.(openapi.GetChannel404ApplicationProblemPlusJSONResponse); !absent {
		t.Fatalf("response = %T, want 404", resp)
	}
}

func TestGetChannel_noAdministrationStore_failsLikeItsNeighbour(t *testing.T) {
	t.Parallel()
	s := NewServer(ledger.NewMemory(), "test")
	_, err := s.GetChannel(as(domain.RoleCurator), openapi.GetChannelRequestObject{Name: "acme-slack"})
	if !errors.Is(err, errNoAdministration) {
		t.Fatalf("err = %v, want errNoAdministration", err)
	}
}

func TestGetMCPServer_noAdministrationStore_failsLikeItsNeighbour(t *testing.T) {
	t.Parallel()
	s := NewServer(ledger.NewMemory(), "test")
	_, err := s.GetMCPServer(as(domain.RoleCurator), openapi.GetMCPServerRequestObject{Name: "github"})
	if !errors.Is(err, errNoAdministration) {
		t.Fatalf("err = %v, want errNoAdministration", err)
	}
}
