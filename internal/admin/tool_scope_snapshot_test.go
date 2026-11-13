package admin_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
)

/*
A listed tool's scope is read with the tool, in one snapshot (NT-019).

The tools and where their servers are stored were two reads. A server removed
between them made its tool read as the installation's, and one stored again
under the same name in a sibling area handed the tool to that area: a
target-scoped answer could show one area's tool to another, and stop matching
what the Gate lets a run reach.
*/
func TestCuratorTools_aServerMovedBetweenTheReads_keepsTheScopeTheToolWasReadWith(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()
	clean := func() {
		_, _ = pool.Exec(context.Background(), `
			delete from settings where (kind = 'mcp_server' and name = 'race-jira') or (kind = 'tool' and name = 'race-jira.search')`)
	}
	clean()
	t.Cleanup(clean)
	if _, err := pool.Exec(ctx, `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		values ('area', 'acme', 'cx', 'mcp_server', 'race-jira', '{"transport":"http","url":"https://mcp.example.com/race"}', true, 'w'),
		       ('installation', '', '', 'tool', 'race-jira.search', '{"server":"race-jira","onSurface":true}', true, 'w')`); err != nil {
		t.Fatalf("plant: %v", err)
	}
	curator := admin.NewCurator(pool)
	curator.BetweenToolReads(func() {
		if _, err := pool.Exec(context.Background(), `
			delete from settings where kind = 'mcp_server' and name = 'race-jira';
			insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
			values ('area', 'acme', 'finance', 'mcp_server', 'race-jira', '{"transport":"http","url":"https://mcp.example.com/race"}', true, 'w')`); err != nil {
			t.Errorf("move the server: %v", err)
		}
	})

	entries, err := curator.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	for _, e := range entries {
		if e.ID == "race-jira.search" && e.Scope != (domain.Scope{Company: "acme", Area: "cx"}) {
			t.Errorf("race-jira.search placed at %s, want acme/cx, the scope it was read with", e.Scope)
		}
	}
}
