package httpapi

import (
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Moving a server out of the installation (NT-019): the platform's door, made
// against the revision read, answering each refusal by its own code.

func moveRequest(ifMatch *string, body openapi.MCPServerMove) openapi.MoveMCPServerRequestObject {
	return openapi.MoveMCPServerRequestObject{
		Name: "wiki", Params: openapi.MoveMCPServerParams{IfMatch: ifMatch}, Body: &body,
	}
}

func TestMoveMCPServer_thePlatformMovesItToAnArea(t *testing.T) {
	t.Parallel()

	fake := &fakeAdmin{moveRevision: `"v1-moved"`}
	resp, err := serverWith(t, fake).MoveMCPServer(as(domain.RoleAdmin),
		moveRequest(ptr(`"v1-read"`), openapi.MCPServerMove{Company: "acme", Area: ptr("cx"), Strand: ptr(true)}))
	if err != nil {
		t.Fatalf("MoveMCPServer: %v", err)
	}
	moved, ok := resp.(openapi.MoveMCPServer204Response)
	if !ok || moved.Headers.ETag != `"v1-moved"` {
		t.Fatalf("response = %#v, want 204 with the new revision", resp)
	}
	m := fake.moved
	if m == nil || m.Name != "wiki" || m.To != (domain.Scope{Company: "acme", Area: "cx"}) || !m.Strand ||
		!m.Expect.Allows(true, `"v1-read"`) || m.Expect.Allows(true, `"v1-other"`) || m.Audit != adminScope {
		t.Fatalf("move = %+v, want wiki to acme/cx, stranding, against v1-read, recorded at the platform", m)
	}
}

// Moving a server out of the installation takes it from every other area,
// so an area's administrator may not.
func TestMoveMCPServer_anAreasAdministrator_isRefused(t *testing.T) {
	t.Parallel()

	fake := &fakeAdmin{}
	resp, err := serverWith(t, fake).MoveMCPServer(inArea("cx", domain.RoleAdmin),
		moveRequest(ptr(`"v1-read"`), openapi.MCPServerMove{Company: "acme", Area: ptr("cx")}))
	if err != nil {
		t.Fatalf("MoveMCPServer: %v", err)
	}
	if _, refused := resp.(openapi.MoveMCPServer403ApplicationProblemPlusJSONResponse); !refused || fake.moved != nil {
		t.Fatalf("response = %T, moved = %v; want 403 and nothing asked", resp, fake.moved)
	}
}

func TestMoveMCPServer_refusesBeforeAskingTheStore(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		ifMatch *string
		body    openapi.MCPServerMove
		want    func(openapi.MoveMCPServerResponseObject) bool
	}{
		{"no If-Match", nil, openapi.MCPServerMove{Company: "acme"},
			func(r openapi.MoveMCPServerResponseObject) bool {
				_, ok := r.(openapi.MoveMCPServer428ApplicationProblemPlusJSONResponse)
				return ok
			}},
		{"a target in another form", ptr(`"v1"`), openapi.MCPServerMove{Company: "Acme", Area: ptr("cx")},
			func(r openapi.MoveMCPServerResponseObject) bool {
				_, ok := r.(openapi.MoveMCPServer400ApplicationProblemPlusJSONResponse)
				return ok
			}},
		{"the installation as a target", ptr(`"v1"`), openapi.MCPServerMove{Company: "*"},
			func(r openapi.MoveMCPServerResponseObject) bool {
				_, ok := r.(openapi.MoveMCPServer400ApplicationProblemPlusJSONResponse)
				return ok
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeAdmin{}
			resp, err := serverWith(t, fake).MoveMCPServer(as(domain.RoleAdmin), moveRequest(tc.ifMatch, tc.body))
			if err != nil {
				t.Fatalf("MoveMCPServer: %v", err)
			}
			if !tc.want(resp) || fake.moved != nil {
				t.Fatalf("response = %#v, moved = %v; want the refusal and nothing asked", resp, fake.moved)
			}
		})
	}
}

func TestMoveMCPServer_answersEachRefusalOfTheStoreByItsCode(t *testing.T) {
	t.Parallel()

	stranded := &admin.MoveStrandsAgents{Agents: []admin.StrandedAgent{
		{Agent: "triage", Scope: domain.Scope{Company: "acme", Area: "ops"}},
	}}
	cases := []struct {
		name string
		err  error
		want func(openapi.MoveMCPServerResponseObject) bool
	}{
		{"agents it would strand", stranded, func(r openapi.MoveMCPServerResponseObject) bool {
			c, ok := r.(openapi.MoveMCPServer409ApplicationProblemPlusJSONResponse)
			return ok && c.Type != nil && *c.Type == "fuseone:move-strands-agents" && c.Agents != nil &&
				len(*c.Agents) == 1 && (*c.Agents)[0] == openapi.StrandedAgent{AgentId: "triage", Company: "acme", Area: "ops"}
		}},
		{"a server outside the installation", admin.ErrMoveNotAtInstallation, func(r openapi.MoveMCPServerResponseObject) bool {
			c, ok := r.(openapi.MoveMCPServer409ApplicationProblemPlusJSONResponse)
			return ok && c.Type != nil && *c.Type == "fuseone:server-not-at-installation"
		}},
		{"an origin out of policy", &admin.EgressRefusal{Origin: "https://wiki.example.com"}, func(r openapi.MoveMCPServerResponseObject) bool {
			b, ok := r.(openapi.MoveMCPServer400ApplicationProblemPlusJSONResponse)
			return ok && b.Type != nil && *b.Type == string(CodeEgressNotAllowed) && b.Detail != nil && *b.Detail == "https://wiki.example.com"
		}},
		{"a stale revision", &domain.StaleError{Current: `"v1-now"`}, func(r openapi.MoveMCPServerResponseObject) bool {
			_, ok := r.(openapi.MoveMCPServer412ApplicationProblemPlusJSONResponse)
			return ok
		}},
		{"no such server", admin.ErrMCPServerUnknown, func(r openapi.MoveMCPServerResponseObject) bool {
			_, ok := r.(openapi.MoveMCPServer404ApplicationProblemPlusJSONResponse)
			return ok
		}},
		{"an unregistered target", admin.ErrMoveTargetUnregistered, func(r openapi.MoveMCPServerResponseObject) bool {
			_, ok := r.(openapi.MoveMCPServer400ApplicationProblemPlusJSONResponse)
			return ok
		}},
		{"a local server", admin.ErrMoveLocalServer, func(r openapi.MoveMCPServerResponseObject) bool {
			_, ok := r.(openapi.MoveMCPServer400ApplicationProblemPlusJSONResponse)
			return ok
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp, err := serverWith(t, &fakeAdmin{moveErr: tc.err}).MoveMCPServer(as(domain.RoleAdmin),
				moveRequest(ptr(`"v1"`), openapi.MCPServerMove{Company: "acme", Area: ptr("cx")}))
			if err != nil {
				t.Fatalf("MoveMCPServer: %v", err)
			}
			if !tc.want(resp) {
				t.Fatalf("response = %#v", resp)
			}
		})
	}
	if errors.Is(stranded, admin.ErrMoveNotAtInstallation) {
		t.Fatal("the two conflicts must stay distinguishable")
	}
}
