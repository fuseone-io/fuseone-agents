package settings_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/vault"
)

/*
An integration's name is unique per kind across the installation (NT-019).

A tool id and an inbound message carry the name and no scope, so two scopes
holding one name would be two things answering to one identity. The database
holds the rule, so every binary is bound by it, the previous one included.
*/

// beforeNames is the last migration before the names became unique.
const beforeNames = "0107_version_claims_enforced"

// previousUpsert is settings.PutTx as every release before this one wrote it.
// The value is a remote server's: an area's MCP server is http (0110), and for
// every other kind the field is noise.
const previousUpsert = `
	insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by, updated_at)
	values ($1, $2, $3, $4, $5, '{"transport":"http"}'::jsonb, true, 'usr_old', now())
	on conflict (scope_kind, company_id, area_id, kind, name) do update set
		value = excluded.value, enabled = excluded.enabled,
		updated_by = excluded.updated_by, updated_at = now()`

// unique are the kinds whose names are identities.
var unique = []settings.Kind{
	settings.KindMCPServer, settings.KindModelProvider, settings.KindConnectorInstance,
	"channel", "channel_conversation",
}

func plant(t *testing.T, pool *pgxpool.Pool, kind settings.Kind, kindOfScope, company, area, name string) error {
	t.Helper()
	_, err := pool.Exec(context.Background(), previousUpsert, kindOfScope, company, area, string(kind), name)
	return err
}

func TestNames_previousBinaryTakingANameAnotherScopeHolds_isRefusedByTheDatabase(t *testing.T) {
	pool := testdb.Isolated(t)
	for _, kind := range unique {
		t.Run(string(kind), func(t *testing.T) {
			if err := plant(t, pool, kind, "area", "acme", "cx", "prod"); err != nil {
				t.Fatalf("first holder: %v", err)
			}
			if err := plant(t, pool, kind, "area", "acme", "finance", "prod"); err == nil {
				t.Fatalf("%s: a second scope took the name", kind)
			}
			// The same scope writing its own row again is still the upsert it was.
			if err := plant(t, pool, kind, "area", "acme", "cx", "prod"); err != nil {
				t.Errorf("rewriting the holder's own row: %v", err)
			}
		})
	}
}

func TestNames_kindsThatRepeatANameAcrossScopes_stayFree(t *testing.T) {
	pool := testdb.Isolated(t)
	// Two rulings of one tool, the owner's and the platform's (NT-019), and a
	// ceiling per scope: the same name by design.
	for _, kind := range []settings.Kind{"tool_classification", settings.KindBudget} {
		if err := plant(t, pool, kind, "installation", "", "", "jira.search"); err != nil {
			t.Fatalf("%s at the installation: %v", kind, err)
		}
		if err := plant(t, pool, kind, "area", "acme", "cx", "jira.search"); err != nil {
			t.Errorf("%s repeated in an area: %v", kind, err)
		}
	}
}

func TestNames_storeTakingANameAnotherScopeHolds_answersErrNameTaken(t *testing.T) {
	pool := testdb.Isolated(t)
	store := settings.NewStore(pool, testVault(t))
	ctx := context.Background()
	at := func(area domain.AreaID) settings.Setting {
		return settings.Setting{
			ScopeKind: settings.ScopeArea, Scope: domain.Scope{Company: "acme", Area: area},
			Kind: settings.KindConnectorInstance, Name: "prod", Enabled: true,
		}
	}
	if err := store.Put(ctx, at("cx")); err != nil {
		t.Fatalf("first holder: %v", err)
	}
	err := store.Put(ctx, at("finance"))
	if !errors.Is(err, settings.ErrNameTaken) {
		t.Fatalf("err = %v, want ErrNameTaken", err)
	}
	if strings.Contains(err.Error(), "cx") {
		t.Errorf("the error %q names the holder's scope", err)
	}
}

func TestNames_twoScopesRacingForOneName_exactlyOneCommits(t *testing.T) {
	pool := testdb.Isolated(t)
	ctx := context.Background()
	first, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = first.Rollback(ctx) }()
	if _, err := first.Exec(ctx, previousUpsert, "area", "acme", "cx", "mcp_server", "jira"); err != nil {
		t.Fatalf("first, uncommitted: %v", err)
	}

	second := make(chan error, 1)
	go func() { second <- plant(t, pool, settings.KindMCPServer, "area", "acme", "finance", "jira") }()
	waitForALockWait(t, pool)
	if err := first.Commit(ctx); err != nil {
		t.Fatalf("commit the first: %v", err)
	}
	if err := <-second; err == nil {
		t.Fatal("both scopes committed the name")
	}
}

func TestNamesMigration_aDuplicateAlreadyStored_stopsTheUpgradeAndNamesIt(t *testing.T) {
	pool := testdb.IsolatedThrough(t, beforeNames)
	ctx := context.Background()
	// Different connectors, one name: the incompatibility NT-019 decided.
	for _, area := range []string{"cx", "finance"} {
		if err := plant(t, pool, settings.KindConnectorInstance, "area", "acme", area, "prod"); err != nil {
			t.Fatalf("plant %s: %v", area, err)
		}
	}

	err := ledger.Migrate(ctx, pool)
	if err == nil {
		t.Fatal("migrated over a duplicate")
	}
	if !strings.Contains(err.Error(), "connector_instance") || !strings.Contains(err.Error(), "prod") {
		t.Errorf("err = %v, want it to name the duplicate's kind and name", err)
	}
	var indexes int
	if err := pool.QueryRow(ctx,
		`select count(*) from pg_indexes where indexname = 'settings_integration_names'`).Scan(&indexes); err != nil {
		t.Fatalf("look for the index: %v", err)
	}
	if indexes != 0 {
		t.Error("the index exists after a stopped upgrade")
	}
}

// waitForALockWait returns once a backend waits on a lock: the second writer,
// stopped by the first's uncommitted row in the index.
func waitForALockWait(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := pool.QueryRow(context.Background(), `
			select count(*) from pg_stat_activity
			where datname = current_database() and wait_event_type = 'Lock'`).Scan(&waiting); err != nil {
			t.Fatalf("read pg_stat_activity: %v", err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the second writer never waited on the first")
}

func testVault(t *testing.T) *vault.Vault {
	t.Helper()
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	return v
}

/*
An area's MCP server is remote (NT-019, decided 2026-11-13).

A stdio server is a process on the platform's worker. Registered by an area, it
would run that area's commands on shared infrastructure, so only the
installation holds one. The database holds the rule, whatever binary writes.
*/
func TestRemoteOnly_anAreasStdioServer_isRefusedByTheDatabase(t *testing.T) {
	pool := testdb.Isolated(t)
	ctx := context.Background()
	put := func(kindOfScope, company, area, name, value string) error {
		_, err := pool.Exec(ctx, `
			insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
			values ($1, $2, $3, 'mcp_server', $4, $5::jsonb, true, 'usr_old')`,
			kindOfScope, company, area, name, value)
		return err
	}
	for _, refused := range []struct{ kind, company, area, name, value string }{
		{"area", "acme", "cx", "cx-local", `{"transport":"stdio","command":"sh"}`},
		{"company", "acme", "", "acme-local", `{"command":"sh"}`},
	} {
		if err := put(refused.kind, refused.company, refused.area, refused.name, refused.value); err == nil {
			t.Errorf("%s: a %s stdio server was stored", refused.name, refused.kind)
		}
	}
	if err := put("area", "acme", "cx", "cx-remote", `{"transport":"http","url":"https://mcp.example.com"}`); err != nil {
		t.Errorf("an area's remote server: %v", err)
	}
	if err := put("installation", "", "", "platform-local", `{"transport":"stdio","command":"sh"}`); err != nil {
		t.Errorf("the platform's stdio server: %v", err)
	}
}
