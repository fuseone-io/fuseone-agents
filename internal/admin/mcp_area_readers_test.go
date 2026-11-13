package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

/*
The readers of an MCP server know where it is stored (NT-019, phase 2 readers).

No door writes an area's server yet. Every reader learns the row's scope first,
a release before any does, so the binary that serves while areas start writing
never reads an area's server as the platform's: not its credential, not its
scope in the catalogue, not the row it rewrites when an OAuth grant refreshes.
*/

var cxScope = domain.Scope{Company: "acme", Area: "cx"}

type areaServerWorld struct {
	integrations *admin.Integrations
	store        *settings.Store
	pool         *pgxpool.Pool
}

// withAreaServer stores "github" at acme/cx, as phase 2's writers will: the
// platform's write, then the row moved to the area with its secret sealed again
// under the area's scope.
func withAreaServer(t *testing.T) areaServerWorld {
	t.Helper()
	return withAreaServerHolding(t, domain.MCPCredentialPatch{Token: ptr("ghp_area_secret")})
}

func withAreaServerHolding(t *testing.T, patch domain.MCPCredentialPatch) areaServerWorld {
	t.Helper()
	ctx := context.Background()
	pool := openPool(t)
	clean := func() error {
		_, err := pool.Exec(context.Background(),
			`delete from settings where kind in ('mcp_server', 'mcp_user_credential', 'mcp_probe', 'mcp_rediscovery')`)
		return err
	}
	if err := clean(); err != nil {
		t.Fatalf("clean: %v", err)
	}
	// Left behind, the area's "github" is a configured server to every later
	// test in the package: a catalogue publication of "github" is judged
	// against it and dropped.
	t.Cleanup(func() {
		if err := clean(); err != nil {
			t.Errorf("clean up: %v", err)
		}
	})
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	store := settings.NewStore(pool, v)
	i := admin.NewIntegrations(pool, store)
	if err := i.PutMCPServer(ctx, "usr_ana", platform, domain.MCPServer{
		Name: "github", Transport: domain.TransportHTTP,
		URL: "https://api.githubcopilot.com/mcp/", Enabled: true,
	}, patch); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	row, err := store.Reveal(ctx, settings.ScopeInstallation, domain.Scope{}, settings.KindMCPServer, "github")
	if err != nil {
		t.Fatalf("reveal the platform's row: %v", err)
	}
	if err := store.Delete(ctx, settings.ScopeInstallation, domain.Scope{}, settings.KindMCPServer, "github"); err != nil {
		t.Fatalf("delete the platform's row: %v", err)
	}
	if err := store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeArea, Scope: cxScope, Kind: settings.KindMCPServer, Name: "github",
		Value: row.Value, Secret: row.Secret, Enabled: true, UpdatedBy: "usr_ana",
	}); err != nil {
		t.Fatalf("store the area's row: %v", err)
	}
	return areaServerWorld{integrations: i, store: store, pool: pool}
}

func TestMCPServers_anAreasServer_carriesItsScope(t *testing.T) {
	w := withAreaServer(t)
	servers, err := w.integrations.MCPServers(context.Background())
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if len(servers) != 1 || servers[0].Scope != cxScope {
		t.Fatalf("servers = %+v, want github at %s", servers, cxScope)
	}
}

func TestMCPCredentials_anAreasServer_isOpenedAtItsOwnScope(t *testing.T) {
	w := withAreaServer(t)
	creds, err := w.integrations.MCPCredentials(context.Background(), "github")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.Token != "ghp_area_secret" {
		t.Errorf("token = %q, want the area's", creds.Token)
	}
}

func TestMCPServerNamed_anAreasServer_isReadAtItsOwnScope(t *testing.T) {
	w := withAreaServer(t)
	server, err := w.integrations.MCPServerNamed(context.Background(), "github")
	if err != nil {
		t.Fatalf("MCPServerNamed: %v", err)
	}
	if server.Scope != cxScope || server.Credential == nil {
		t.Errorf("server = %+v, want the area's, with its credential summary", server)
	}
}

func TestRequestMCPProbe_anAreasServer_isFound(t *testing.T) {
	w := withAreaServer(t)
	if err := w.integrations.RequestMCPProbe(context.Background(), "usr_ana", platform, "github"); err != nil {
		t.Errorf("RequestMCPProbe: %v", err)
	}
}

func TestRequestMCPRediscovery_anAreasServer_isFound(t *testing.T) {
	w := withAreaServer(t)
	if _, err := w.integrations.RequestMCPRediscovery(context.Background(), "usr_ana", platform, "github"); err != nil {
		t.Errorf("RequestMCPRediscovery: %v", err)
	}
	if _, err := w.integrations.MCPRediscovery(context.Background(), "github"); err != nil {
		t.Errorf("MCPRediscovery: %v", err)
	}
}

// Removing an area's server removes the area's row, and leaves nothing at the
// installation behind or created.
func TestRemoveMCPServer_anAreasServer_removesItsOwnRow(t *testing.T) {
	w := withAreaServer(t)
	ctx := context.Background()
	if err := w.integrations.RemoveMCPServer(ctx, admin.Removal{By: "usr_ana", Scope: platform, Name: "github"}); err != nil {
		t.Fatalf("RemoveMCPServer: %v", err)
	}
	var left int
	if err := w.pool.QueryRow(ctx,
		`select count(*) from settings where kind = 'mcp_server' and name = 'github'`).Scan(&left); err != nil {
		t.Fatalf("count: %v", err)
	}
	if left != 0 {
		t.Errorf("%d rows of github left, want none", left)
	}
}

// An area's server is configured like any other: its worker's discovery is the
// authority on its tools, and publishing them keeps them in the catalogue.
func TestPublish_anAreasServer_isJudgedByItsConfiguration(t *testing.T) {
	curator, pool := catalogueOf(t)
	var generation time.Time
	if err := pool.QueryRow(context.Background(), `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by, updated_at)
		values ('area', 'acme', 'cx', 'mcp_server', 'cx-jira', '{"transport":"http"}', true, 'usr_ana', clock_timestamp())
		returning updated_at`).Scan(&generation); err != nil {
		t.Fatalf("configure the area's server: %v", err)
	}
	t.Cleanup(func() { unconfigure(t, pool, "cx-jira") })

	publish(t, curator, discovered("cx-jira", generation, "cx-jira.search"))

	if got := offered(t, curator, "cx-jira"); len(got) != 1 || got[0] != "cx-jira.search" {
		t.Errorf("offered = %v, want the area's server's tool kept", got)
	}
}

// A ruling says where it is stored, so the catalogue can tell the platform's
// ceiling from the owner's word (NT-019).
func TestCuratorList_aRulingSaysWhereItIsStored(t *testing.T) {
	curator := freshCurator(t)
	pool := openPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `delete from settings where kind = 'tool_classification'`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	for _, row := range []struct{ kind, company, area string }{
		{"installation", "", ""}, {"area", "acme", "cx"},
	} {
		if _, err := pool.Exec(ctx, `
			insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
			values ($1, $2, $3, 'tool_classification', 'jira.search', '{"effect":"read"}', true, 'usr_ana')`,
			row.kind, row.company, row.area); err != nil {
			t.Fatalf("plant the %s ruling: %v", row.kind, err)
		}
	}

	rulings, err := curator.List(ctx, domain.Scope{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := map[domain.Scope]bool{}
	for _, r := range rulings {
		got[r.Scope] = true
	}
	if !got[domain.Scope{Company: domain.Installation}] || !got[cxScope] || len(rulings) != 2 {
		t.Errorf("rulings at %v, want one at the installation and one at %s", got, cxScope)
	}
}

// The console's listing reads a tool's ruling as the worker does: the
// platform's over the owner's, and nobody else's (NT-019).
func TestCuratorTools_readsTheRulingInForce(t *testing.T) {
	curator := freshCurator(t)
	pool := openPool(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`delete from settings where kind in ('tool_classification', 'mcp_server')`,
		`insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		 values ('area', 'acme', 'cx', 'mcp_server', 'cx-jira', '{"transport":"http"}', true, 'usr_ana')`,
		`insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		 values ('installation', '', '', 'tool', 'cx-jira.search', '{"server":"cx-jira","onSurface":true}', true, 'w'),
		        ('installation', '', '', 'tool', 'cx-jira.create', '{"server":"cx-jira","onSurface":true}', true, 'w')`,
		// search: the owner says read, the platform says write.
		`insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		 values ('area', 'acme', 'cx', 'tool_classification', 'cx-jira.search', '{"effect":"read"}', true, 'usr_cx'),
		        ('installation', '', '', 'tool_classification', 'cx-jira.search', '{"effect":"write"}', true, 'usr_ana')`,
		// create: only a ruling stored at a sibling area, which is nobody's word.
		`insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		 values ('area', 'acme', 'finance', 'tool_classification', 'cx-jira.create', '{"effect":"read"}', true, 'usr_fin')`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("plant: %v", err)
		}
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `delete from settings where name like 'cx-jira%'`) })

	entries, err := curator.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	effects := map[domain.ToolID]domain.Effect{}
	for _, e := range entries {
		effects[e.ID] = e.Effect
	}
	if effects["cx-jira.search"] != domain.EffectWrite {
		t.Errorf("search = %v, want the platform's write", effects["cx-jira.search"])
	}
	if effects["cx-jira.create"] != domain.EffectUnknown {
		t.Errorf("create = %v, want unruled: a sibling's ruling is nobody's", effects["cx-jira.create"])
	}
}

/*
An area's server refreshes its OAuth grant where it is stored.

Rewritten at the installation instead, the refresh would add a second row for
one name, which the database refuses (0108): the new grant would be lost and
the server would keep presenting the old one until it expired.
*/
func TestRefreshMCPServerOAuth_anAreasServer_rewritesItsOwnRow(t *testing.T) {
	was := domain.MCPOAuthGrant{
		AccessToken: "old-access", RefreshToken: "old-refresh",
		TokenURL: "https://issuer.example/token", ClientID: "client", ExpiresAtUnix: 100,
	}
	next := was
	next.AccessToken, next.ExpiresAtUnix = "new-access", 200
	w := withAreaServerHolding(t, domain.MCPCredentialPatch{OAuth: &was})
	ctx := context.Background()

	if err := w.integrations.RefreshMCPServerOAuth(ctx, admin.MCPOAuthRefresh{Server: "github", Scope: cxScope, Was: was, Next: next}); err != nil {
		t.Fatalf("RefreshMCPServerOAuth: %v", err)
	}
	creds, err := w.integrations.MCPCredentials(ctx, "github")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.OAuth == nil || creds.OAuth.AccessToken != "new-access" {
		t.Errorf("oauth = %+v, want the refreshed grant on the area's row", creds.OAuth)
	}
	var atInstallation int
	if err := w.pool.QueryRow(ctx, `
		select count(*) from settings where kind = 'mcp_server' and name = 'github' and scope_kind = 'installation'`,
	).Scan(&atInstallation); err != nil {
		t.Fatalf("count: %v", err)
	}
	if atInstallation != 0 {
		t.Errorf("%d installation rows of github, want none", atInstallation)
	}
}
