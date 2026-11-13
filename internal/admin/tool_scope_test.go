package admin_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
A listed tool carries the runtime scope of the server that declares it
(NT-019, phase 2 writers, part B2a).

The reduced answer an author picks from keeps a tool only where a run at the
target reaches it, which is the Gate's checkScope over this same scope. A tool
of a server no row describes, one a --mcp flag connected, is the
installation's, as the worker's catalogue reads it.
*/
func TestCuratorTools_eachToolCarriesItsServersRuntimeScope(t *testing.T) {
	curator := freshCurator(t)
	pool := openPool(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`delete from settings where kind = 'mcp_server'`,
		`insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		 values ('area', 'acme', 'cx', 'mcp_server', 'cx-jira', '{"transport":"http"}', true, 'usr_ana'),
		        ('company', 'acme', '', 'mcp_server', 'acme-wiki', '{"transport":"http"}', true, 'usr_ana'),
		        ('installation', '', '', 'mcp_server', 'github', '{"transport":"http"}', true, 'usr_ana')`,
		`insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		 values ('installation', '', '', 'tool', 'cx-jira.search', '{"server":"cx-jira","onSurface":true}', true, 'w'),
		        ('installation', '', '', 'tool', 'acme-wiki.read', '{"server":"acme-wiki","onSurface":true}', true, 'w'),
		        ('installation', '', '', 'tool', 'github.search', '{"server":"github","onSurface":true}', true, 'w'),
		        ('installation', '', '', 'tool', 'flagged.ping', '{"server":"flagged","onSurface":true}', true, 'w')`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("plant: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `delete from settings where kind in ('mcp_server', 'tool')`)
	})

	entries, err := curator.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	got := map[domain.ToolID]domain.Scope{}
	for _, e := range entries {
		got[e.ID] = e.Scope
	}
	installation := domain.Scope{Company: domain.Installation}
	for tool, want := range map[domain.ToolID]domain.Scope{
		"cx-jira.search": cxScope,
		"acme-wiki.read": {Company: "acme"},
		"github.search":  installation,
		"flagged.ping":   installation,
	} {
		if got[tool] != want {
			t.Errorf("%s carries %+v, want %+v", tool, got[tool], want)
		}
	}
}
