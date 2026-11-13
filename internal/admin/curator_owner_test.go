package admin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/testdb"
)

/*
An owner classifies the tools of its own servers, under the platform's ceiling
(NT-019, phase 2 writers).

The owner's ruling is stored at the scope where the tool's server is stored,
and only on the exact row the door read: a server removed and stored again in
another scope in between does not receive it. The platform's ruling stays at
the installation, and withdrawing it hands the tool back to the owner's.
*/

// ownedWorld is a database holding cx's server cx-jira, the installation's
// github, and one published tool of each.
func ownedWorld(t *testing.T) (*pgxpool.Pool, *admin.Curator) {
	t.Helper()
	pool := testdb.Isolated(t)
	for _, stmt := range []string{
		`insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		 values ('area', 'acme', 'cx', 'mcp_server', 'cx-jira', '{"transport":"http"}', true, 'usr_ana'),
		        ('installation', '', '', 'mcp_server', 'github', '{"transport":"http"}', true, 'usr_ana')`,
		`insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		 values ('installation', '', '', 'tool', 'cx-jira.search', '{"server":"cx-jira","onSurface":true}', true, 'w'),
		        ('installation', '', '', 'tool', 'github.search', '{"server":"github","onSurface":true}', true, 'w'),
		        ('installation', '', '', 'tool', 'flagged.search', '{"server":"flagged","onSurface":true}', true, 'w')`,
	} {
		if _, err := pool.Exec(t.Context(), stmt); err != nil {
			t.Fatalf("plant: %v", err)
		}
	}
	return pool, admin.NewCurator(pool)
}

// rulingRows is where each stored ruling on tool lives, as scope_kind/company/area.
func rulingRows(t *testing.T, pool *pgxpool.Pool, tool string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		select scope_kind || '/' || company_id || '/' || area_id from settings
		where kind = 'tool_classification' and name = $1 order by 1`, tool)
	if err != nil {
		t.Fatalf("read rulings: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var at string
		if err := rows.Scan(&at); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, at)
	}
	return out
}

func ownerOf(t *testing.T, c *admin.Curator, tool domain.ToolID) admin.ToolOwner {
	t.Helper()
	owner, err := c.OwnerOf(context.Background(), tool)
	if err != nil {
		t.Fatalf("OwnerOf %s: %v", tool, err)
	}
	return owner
}

func TestClassifyAsOwner_storesTheRulingAtTheServersScope(t *testing.T) {
	pool, c := ownedWorld(t)
	ctx := context.Background()
	owner := ownerOf(t, c, "cx-jira.search")
	if owner.Server != "cx-jira" || owner.At.Kind != settings.ScopeArea || owner.At.Stored != cxScope {
		t.Fatalf("owner = %+v, want cx-jira stored at the area %s", owner, cxScope)
	}
	if err := c.ClassifyAsOwner(ctx, owner, domain.ToolClassification{
		Tool: "cx-jira.search", Effect: domain.EffectRead, By: "usr_cx",
	}); err != nil {
		t.Fatalf("ClassifyAsOwner: %v", err)
	}
	if got := rulingRows(t, pool, "cx-jira.search"); len(got) != 1 || got[0] != "area/acme/cx" {
		t.Errorf("rulings at %v, want one at area/acme/cx", got)
	}
	var company, area string
	if err := pool.QueryRow(ctx, `
		select company_id, area_id from admin_events where action = 'tool.classified' and target = 'cx-jira.search'`,
	).Scan(&company, &area); err != nil {
		t.Fatalf("the trail: %v", err)
	}
	if company != "acme" || area != "cx" {
		t.Errorf("recorded at %s/%s, want the area's trail", company, area)
	}
	if e := find(t, toolsOf(t, c), "cx-jira.search"); e.Effect != domain.EffectRead {
		t.Errorf("listed effect = %v, want the owner's read", e.Effect)
	}
}

// The platform's door is today's, wherever the tool's server is stored.
func TestClassify_thePlatformsRuling_staysAtTheInstallation(t *testing.T) {
	pool, c := ownedWorld(t)
	if err := c.Classify(context.Background(), platform, domain.ToolClassification{
		Tool: "cx-jira.search", Effect: domain.EffectWrite, By: "usr_ana",
	}); err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got := rulingRows(t, pool, "cx-jira.search"); len(got) != 1 || got[0] != "installation//" {
		t.Errorf("rulings at %v, want one at the installation", got)
	}
}

// For a server of the installation the owner's row is the platform's: one ruling.
func TestClassifyAsOwner_aServerOfTheInstallation_writesThePlatformsRow(t *testing.T) {
	pool, c := ownedWorld(t)
	owner := ownerOf(t, c, "github.search")
	if owner.At.Kind != settings.ScopeInstallation {
		t.Fatalf("owner = %+v, want the installation", owner)
	}
	if err := c.ClassifyAsOwner(context.Background(), owner, domain.ToolClassification{
		Tool: "github.search", Effect: domain.EffectRead, By: "usr_ana",
	}); err != nil {
		t.Fatalf("ClassifyAsOwner: %v", err)
	}
	if got := rulingRows(t, pool, "github.search"); len(got) != 1 || got[0] != "installation//" {
		t.Errorf("rulings at %v, want the one platform row", got)
	}
}

// A tool nobody published, or whose server no row holds (a --mcp flag's), has
// no owner scope to store a ruling at.
func TestOwnerOf_aToolWithNoStoredServer_hasNoOwnerScope(t *testing.T) {
	_, c := ownedWorld(t)
	for _, tool := range []domain.ToolID{"flagged.search", "nobody.published"} {
		if _, err := c.OwnerOf(context.Background(), tool); !errors.Is(err, admin.ErrNoOwnerScope) {
			t.Errorf("OwnerOf %s = %v, want ErrNoOwnerScope", tool, err)
		}
	}
}

/*
The ruling lands on the row the door read, or nowhere.

Read at acme/cx; then the server is removed and acme/finance stores one of the
same name. The ruling the cx curator was authorized for must not become the
finance server's owner ruling.
*/
func TestClassifyAsOwner_aServerMovedSinceTheLookup_getsNoRuling(t *testing.T) {
	pool, c := ownedWorld(t)
	ctx := context.Background()
	owner := ownerOf(t, c, "cx-jira.search")
	if _, err := pool.Exec(ctx, `
		delete from settings where kind = 'mcp_server' and name = 'cx-jira';
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		values ('area', 'acme', 'finance', 'mcp_server', 'cx-jira', '{"transport":"http"}', true, 'usr_fin')`); err != nil {
		t.Fatalf("move the server: %v", err)
	}
	err := c.ClassifyAsOwner(ctx, owner, domain.ToolClassification{
		Tool: "cx-jira.search", Effect: domain.EffectRead, By: "usr_cx",
	})
	if !errors.Is(err, admin.ErrMCPServerUnknown) {
		t.Errorf("ClassifyAsOwner = %v, want ErrMCPServerUnknown", err)
	}
	if got := rulingRows(t, pool, "cx-jira.search"); len(got) != 0 {
		t.Errorf("rulings at %v, want none", got)
	}
}

/*
The row is held, not only looked up, inside the write's transaction.

A removal of cx's server is in flight when the ruling arrives, and commits with
finance's server of the same name in its place. Read without a lock, the
removal's uncommitted delete is invisible and the ruling lands at acme/cx for a
server that is gone; held, the write waits and finds the row gone.
*/
func TestClassifyAsOwner_aRemovalInFlight_isWaitedFor(t *testing.T) {
	pool, c := ownedWorld(t)
	ctx := context.Background()
	owner := ownerOf(t, c, "cx-jira.search")
	holder, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = holder.Rollback(ctx) }()
	if _, err := holder.Exec(ctx, `delete from settings where kind = 'mcp_server' and name = 'cx-jira'`); err != nil {
		t.Fatalf("remove the server: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- c.ClassifyAsOwner(ctx, owner, domain.ToolClassification{
			Tool: "cx-jira.search", Effect: domain.EffectRead, By: "usr_cx",
		})
	}()
	select {
	case err := <-done:
		t.Fatalf("the ruling was written (%v) while the server's removal was in flight", err)
	case <-waitForRowLock(t, pool):
	case <-time.After(10 * time.Second):
		t.Fatal("the ruling neither finished nor came to wait on the server's row")
	}
	if _, err := holder.Exec(ctx, `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		values ('area', 'acme', 'finance', 'mcp_server', 'cx-jira', '{"transport":"http"}', true, 'usr_fin')`); err != nil {
		t.Fatalf("store finance's server: %v", err)
	}
	if err := holder.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := <-done; !errors.Is(err, admin.ErrMCPServerUnknown) {
		t.Errorf("ClassifyAsOwner = %v, want ErrMCPServerUnknown", err)
	}
	if got := rulingRows(t, pool, "cx-jira.search"); len(got) != 0 {
		t.Errorf("rulings at %v, want none", got)
	}
}

// waitForRowLock closes once a backend waits on a row's lock, and never
// closes otherwise: the caller's other case decides.
func waitForRowLock(t *testing.T, pool *pgxpool.Pool) <-chan struct{} {
	t.Helper()
	waiting := make(chan struct{})
	go func() {
		for range 500 {
			var n int
			if err := pool.QueryRow(context.Background(), `
				select count(*) from pg_stat_activity
				where datname = current_database() and wait_event_type = 'Lock'`).Scan(&n); err != nil {
				return
			}
			if n > 0 {
				close(waiting)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	return waiting
}

// An owner's ruling names a tool of the server it was authorized for, and no
// other server's.
func TestClassifyAsOwner_aToolOfAnotherServer_isRefused(t *testing.T) {
	pool, c := ownedWorld(t)
	owner := ownerOf(t, c, "cx-jira.search")
	err := c.ClassifyAsOwner(context.Background(), owner, domain.ToolClassification{
		Tool: "github.search", Effect: domain.EffectRead, By: "usr_cx",
	})
	if err == nil {
		t.Error("an owner's ruling for cx-jira was stored on github's tool")
	}
	if got := rulingRows(t, pool, "github.search"); len(got) != 0 {
		t.Errorf("rulings at %v, want none", got)
	}
}

func TestWithdraw_thePlatformsRuling_leavesTheOwnersInForce(t *testing.T) {
	pool, c := ownedWorld(t)
	ctx := context.Background()
	if err := c.ClassifyAsOwner(ctx, ownerOf(t, c, "cx-jira.search"), domain.ToolClassification{
		Tool: "cx-jira.search", Effect: domain.EffectRead, By: "usr_cx",
	}); err != nil {
		t.Fatalf("ClassifyAsOwner: %v", err)
	}
	if err := c.Classify(ctx, platform, domain.ToolClassification{
		Tool: "cx-jira.search", Effect: domain.EffectWrite, By: "usr_ana",
	}); err != nil {
		t.Fatalf("Classify: %v", err)
	}
	listed := find(t, toolsOf(t, c), "cx-jira.search")
	if listed.Effect != domain.EffectWrite || len(listed.Rulings) != 2 || !listed.Rulings[1].Overridden {
		t.Fatalf("listed %v with %+v, want the platform's write and the owner's overridden", listed.Effect, listed.Rulings)
	}

	if err := c.Withdraw(ctx, "usr_ana", platform, "cx-jira.search"); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if got := rulingRows(t, pool, "cx-jira.search"); len(got) != 1 || got[0] != "area/acme/cx" {
		t.Errorf("rulings at %v, want only the owner's", got)
	}
	listed = find(t, toolsOf(t, c), "cx-jira.search")
	if listed.Effect != domain.EffectRead || len(listed.Rulings) != 1 || !listed.Rulings[0].InForce {
		t.Errorf("listed %v with %+v, want the owner's read in force", listed.Effect, listed.Rulings)
	}
	var recorded int
	if err := pool.QueryRow(ctx, `
		select count(*) from admin_events where action = 'tool.classification_withdrawn' and target = 'cx-jira.search'`,
	).Scan(&recorded); err != nil || recorded != 1 {
		t.Errorf("withdrawals recorded = %d (%v), want 1", recorded, err)
	}
	if err := c.Withdraw(ctx, "usr_ana", platform, "cx-jira.search"); !errors.Is(err, admin.ErrRulingUnknown) {
		t.Errorf("a second Withdraw = %v, want ErrRulingUnknown", err)
	}
}

func toolsOf(t *testing.T, c *admin.Curator) []domain.ToolEntry {
	t.Helper()
	entries, err := c.Tools(context.Background())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	return entries
}
