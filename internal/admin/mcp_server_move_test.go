package admin_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

/*
Moving a server out of the installation (NT-019, "Moving a server out of the
installation"). A server stored at the installation before areas had their
own is moved to the area that owns it: same name, configuration, secret and
tools, at another scope, with its secret sealed again there and every refusal
answered before anything is written.
*/

const movable = "movable"

type moveWorld struct {
	integrations *admin.Integrations
	store        *settings.Store
	pool         *pgxpool.Pool
	revision     domain.Revision
}

// withMovableServer stores "movable" at the installation, remote, with a
// token, and registers acme and acme/cx. allow lists the server's origin in
// acme/cx's egress policy.
func withMovableServer(t *testing.T, allow bool) moveWorld {
	t.Helper()
	ctx := context.Background()
	pool := openPool(t)
	clean := func() error {
		_, err := pool.Exec(context.Background(), `delete from settings
			where kind in ('mcp_server', 'mcp_user_credential', 'mcp_egress_policy', 'mcp_probe', 'mcp_rediscovery')`)
		return err
	}
	if err := clean(); err != nil {
		t.Fatalf("clean: %v", err)
	}
	t.Cleanup(func() {
		if err := clean(); err != nil {
			t.Errorf("clean up: %v", err)
		}
	})
	if _, err := pool.Exec(ctx, `
		insert into companies (company_id, name) values ('acme', 'Acme') on conflict do nothing;
		insert into scopes (company_id, area_id, label) values ('acme', 'cx', 'CX') on conflict do nothing`); err != nil {
		t.Fatalf("register acme/cx: %v", err)
	}
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	store := settings.NewStore(pool, v)
	i := admin.NewIntegrations(pool, store)
	rev, err := i.WriteMCPServer(ctx, admin.MCPServerWrite{
		By: "usr_ana", Scope: platform,
		Server: domain.MCPServer{Name: movable, Transport: domain.TransportHTTP, URL: "https://mcp.example.com/mcp", Enabled: true},
		Given:  domain.MCPCredentialPatch{Token: ptr("tok_movable")},
	})
	if err != nil {
		t.Fatalf("WriteMCPServer: %v", err)
	}
	if allow {
		if err := i.PutMCPEgressPolicy(ctx, "usr_ana", platform, admin.MCPEgressPolicy{
			Scope: cxScope, Origins: []string{"https://mcp.example.com"},
		}); err != nil {
			t.Fatalf("PutMCPEgressPolicy: %v", err)
		}
	}
	return moveWorld{integrations: i, store: store, pool: pool, revision: rev}
}

func (w moveWorld) move(t *testing.T, m admin.MCPServerMove) (domain.Revision, error) {
	t.Helper()
	return w.moveAs(t, m, true)
}

// moveAs fills what the case leaves out; withIfMatch false sends none.
func (w moveWorld) moveAs(t *testing.T, m admin.MCPServerMove, withIfMatch bool) (domain.Revision, error) {
	t.Helper()
	if m.Name == "" {
		m.Name = movable
	}
	if m.To == (domain.Scope{}) {
		m.To = cxScope
	}
	if withIfMatch && !m.Expect.Present() {
		m.Expect = ifMatch(t, w.revision)
	}
	m.By, m.Audit = "usr_ana", platform
	return w.integrations.MoveMCPServer(context.Background(), m)
}

func ifMatch(t *testing.T, rev domain.Revision) domain.IfMatch {
	t.Helper()
	header := string(rev)
	m, err := domain.ParseIfMatch(&header)
	if err != nil {
		t.Fatalf("ParseIfMatch(%s): %v", rev, err)
	}
	return m
}

func TestMoveMCPServer_toAnArea_storesItThereWithItsSecretOpenedAtTheArea(t *testing.T) {
	w := withMovableServer(t, true)
	if _, err := w.move(t, admin.MCPServerMove{}); err != nil {
		t.Fatalf("MoveMCPServer: %v", err)
	}

	servers, err := w.integrations.MCPServers(context.Background())
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if len(servers) != 1 || servers[0].Scope != cxScope {
		t.Fatalf("servers = %+v, want %s at %s alone", servers, movable, cxScope)
	}
	creds, err := w.integrations.MCPCredentials(context.Background(), movable)
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.Token != "tok_movable" {
		t.Errorf("token = %q, want the one sealed at the installation, opened at the area", creds.Token)
	}
}

// Both trails show it: the platform's, where it left, and the area's, where
// it arrived.
func TestMoveMCPServer_isRecordedAtBothScopes(t *testing.T) {
	w := withMovableServer(t, true)
	count := func(at domain.Scope) int {
		var n int
		if err := w.pool.QueryRow(context.Background(), `
			select count(*) from admin_events
			where action = 'mcp_server.moved' and target = $1 and company_id = $2 and area_id = $3`,
			movable, string(at.Company), string(at.Area)).Scan(&n); err != nil {
			t.Fatalf("count events: %v", err)
		}
		return n
	}
	before := map[domain.Scope]int{platform: count(platform), cxScope: count(cxScope)}
	if _, err := w.move(t, admin.MCPServerMove{}); err != nil {
		t.Fatalf("MoveMCPServer: %v", err)
	}
	for at, was := range before {
		if got := count(at) - was; got != 1 {
			t.Errorf("events recorded at %s = %d, want 1", at, got)
		}
	}
}

// A person's credential is stored at the installation and names its server's
// row; the move points it at the new row, or the worker hands it nothing.
func TestMoveMCPServer_carriesThePersonalCredentials(t *testing.T) {
	w := withMovableServer(t, true)
	if err := w.integrations.PutMCPPersonalCredential(context.Background(), "usr_bia", domain.Scope{Company: "acme", Area: "cx"},
		movable, domain.MCPCredentialPatch{Token: ptr("tok_bia")}); err != nil {
		t.Fatalf("PutMCPPersonalCredential: %v", err)
	}
	if _, err := w.move(t, admin.MCPServerMove{}); err != nil {
		t.Fatalf("MoveMCPServer: %v", err)
	}
	creds, ok, err := w.integrations.MCPPersonalCredential(context.Background(), cxScope, movable, "usr_bia")
	if err != nil || !ok || creds.Token != "tok_bia" {
		t.Fatalf("personal credential = %+v, %v, %v; want Bia's token handed at the area", creds, ok, err)
	}
}

func TestMoveMCPServer_refusesBeforeWritingAnything(t *testing.T) {
	cases := []struct {
		name      string
		allow     bool
		noIfMatch bool
		setup     func(t *testing.T, w moveWorld) admin.MCPServerMove
		is        func(error) bool
	}{
		{
			name: "a stale If-Match", allow: true,
			setup: func(t *testing.T, _ moveWorld) admin.MCPServerMove {
				return admin.MCPServerMove{Expect: ifMatch(t, `"v1-stale"`)}
			},
			is: func(err error) bool { var s *domain.StaleError; return errors.As(err, &s) },
		},
		{
			name: "no If-Match", allow: true, noIfMatch: true,
			setup: func(*testing.T, moveWorld) admin.MCPServerMove { return admin.MCPServerMove{} },
			is:    func(err error) bool { return errors.Is(err, admin.ErrMoveNeedsIfMatch) },
		},
		{
			name: "a target nobody registered", allow: true,
			setup: func(*testing.T, moveWorld) admin.MCPServerMove {
				return admin.MCPServerMove{To: domain.Scope{Company: "acme", Area: "ghost"}}
			},
			is: func(err error) bool { return errors.Is(err, admin.ErrMoveTargetUnregistered) },
		},
		{
			name: "an origin the target's policy does not list", allow: false,
			setup: func(*testing.T, moveWorld) admin.MCPServerMove { return admin.MCPServerMove{} },
			is:    func(err error) bool { var r *admin.EgressRefusal; return errors.As(err, &r) },
		},
		{
			name: "a server already outside the installation", allow: true,
			setup: func(t *testing.T, w moveWorld) admin.MCPServerMove {
				rev, err := w.move(t, admin.MCPServerMove{})
				if err != nil {
					t.Fatalf("first move: %v", err)
				}
				return admin.MCPServerMove{Expect: ifMatch(t, rev)}
			},
			is: func(err error) bool { return errors.Is(err, admin.ErrMoveNotAtInstallation) },
		},
		{
			name: "a local server", allow: true,
			setup: func(t *testing.T, w moveWorld) admin.MCPServerMove {
				rev, err := w.integrations.WriteMCPServer(context.Background(), admin.MCPServerWrite{
					By: "usr_ana", Scope: platform,
					Server: domain.MCPServer{Name: "reports", Transport: domain.TransportStdio, Command: "/usr/bin/reports",
						Enabled: true, AcceptsLocalExecution: true},
				})
				if err != nil {
					t.Fatalf("store a local server: %v", err)
				}
				return admin.MCPServerMove{Name: "reports", Expect: ifMatch(t, rev)}
			},
			is: func(err error) bool { return errors.Is(err, admin.ErrMoveLocalServer) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := withMovableServer(t, tc.allow)
			m := tc.setup(t, w)
			before := storedAt(t, w.pool)
			_, err := w.moveAs(t, m, !tc.noIfMatch)
			if err == nil || !tc.is(err) {
				t.Fatalf("MoveMCPServer = %v, want the refusal", err)
			}
			if after := storedAt(t, w.pool); after != before {
				t.Errorf("rows = %q, want them as they were: %q", after, before)
			}
		})
	}
}

/*
Agents outside the target that name the server's tools would be blocked by
the Gate after the move. The move names them and refuses, unless the request
says strand. A snapshot: what is published when the move reads.
*/
func TestMoveMCPServer_agentsItWouldStrand_areNamedAndRefused(t *testing.T) {
	w := withMovableServer(t, true)
	ops := plantAgent(t, w.pool, "acme", "ops", movable+".lookup")
	inside := plantAgent(t, w.pool, "acme", "cx", movable+".lookup")

	_, err := w.move(t, admin.MCPServerMove{})
	var stranded *admin.MoveStrandsAgents
	if !errors.As(err, &stranded) {
		t.Fatalf("MoveMCPServer = %v, want the agents it would strand", err)
	}
	if len(stranded.Agents) != 1 || stranded.Agents[0].Agent != ops ||
		stranded.Agents[0].Scope != (domain.Scope{Company: "acme", Area: "ops"}) {
		t.Fatalf("stranded = %+v, want %s at acme/ops alone (not %s, inside the target)", stranded.Agents, ops, inside)
	}

	if _, err := w.move(t, admin.MCPServerMove{Strand: true}); err != nil {
		t.Fatalf("MoveMCPServer with strand: %v", err)
	}
}

func plantAgent(t *testing.T, pool *pgxpool.Pool, company, area, tool string) domain.AgentID {
	t.Helper()
	id := domain.AgentID(fmt.Sprintf("move-%s-%d", area, time.Now().UnixNano()))
	// A version presents the digest it claimed first (NT-018).
	if _, err := pool.Exec(context.Background(), `
		insert into agent_version_digests (agent_id, version_id, digest) values ($1, 'v1', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')`,
		string(id)); err != nil {
		t.Fatalf("claim the version: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
		insert into agent_specs (
			agent_id, version_id, company_id, area_id, name,
			provider, model, effort, tools, budget, triggers,
			instructions, source, published_by, emits, steps,
			memory_learning, approvals, digest
		) values ($1, 'v1', $2, $3, 'Moved tools',
			'openai', 'test-model', '', array[$4], '{}'::jsonb, '[]'::jsonb,
			'', 'move.agent.md', 'worker', '[]'::jsonb,
			'[]'::jsonb, '{"mode":"off"}'::jsonb, '{}'::jsonb, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')`,
		string(id), company, area, tool); err != nil {
		t.Fatalf("plant agent: %v", err)
	}
	// Versions are never deleted, so the agent is retired when the test ends:
	// a retired agent strands nothing, and the next test's moves stay free.
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `
			insert into agent_state (agent_id, retired_at) values ($1, now())
			on conflict (agent_id) do update set retired_at = now()`, string(id)); err != nil {
			t.Errorf("retire %s: %v", id, err)
		}
	})
	return id
}

// storedAt names every MCP server row and its scope, to show a refusal wrote
// nothing.
func storedAt(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var out string
	if err := pool.QueryRow(context.Background(), `
		select coalesce(string_agg(name || '@' || scope_kind || ':' || company_id || '/' || area_id || ':' || updated_at::text, ',' order by name), '')
		from settings where kind = 'mcp_server'`).Scan(&out); err != nil {
		t.Fatalf("read rows: %v", err)
	}
	return out
}
