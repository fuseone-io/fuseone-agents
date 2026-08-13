package audit_test

import (
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/audit"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

// The audit trail reads two records that are not the same kind of thing: one
// hash-chained, one append-only by grant. Merging them is only honest if an
// entry says which it came from — and if neither can be read by somebody who
// cannot see the area it belongs to.

var noon = time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

func readerFor(t *testing.T) (*audit.Postgres, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping the audit suite")
	}

	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `truncate run_steps, runs, admin_events`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return audit.NewPostgres(pool), pool
}

func seedDecision(t *testing.T, pool *pgxpool.Pool, at time.Time, verdict int, area string) {
	t.Helper()
	store := ledger.NewPostgres(pool)
	runID := domain.RunID("run-" + area + at.Format("150405"))
	for _, step := range []domain.Step{
		{RunID: runID, Kind: domain.StepRunStarted, At: at.Add(-time.Second),
			Scope:   domain.Scope{Company: "acme", Area: domain.AreaID(area)},
			AgentID: "triage", VersionID: "v1"},
		{RunID: runID, Kind: domain.StepGateDecided, At: at,
			Scope:   domain.Scope{Company: "acme", Area: domain.AreaID(area)},
			AgentID: "triage", VersionID: "v1",
			Payload: []byte(`{"tool":"crm.reply","verdict":` + string(rune('0'+verdict)) + `}`)},
	} {
		if _, err := store.Append(t.Context(), step); err != nil {
			t.Fatalf("seed %s: %v", step.Kind, err)
		}
	}
}

func seedAdmin(t *testing.T, pool *pgxpool.Pool, at time.Time, actor, action, area string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `
		insert into admin_events (at, principal_id, company_id, area_id, action, target, detail)
		values ($1, $2, 'acme', $3, $4, 'crm.reply', '{}'::jsonb)`,
		at, actor, area, action); err != nil {
		t.Fatalf("seed admin event: %v", err)
	}
}

func TestRead_mergesBothRecordsInOneOrderedStream(t *testing.T) {
	reader, pool := readerFor(t)

	seedAdmin(t, pool, noon, "usr_ana", "tool.classified", "cx")
	seedDecision(t, pool, noon.Add(time.Minute), 4, "cx")

	entries, _, err := reader.Read(t.Context(), audit.Filter{}, 50)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want both records", len(entries))
	}
	// Newest first: an audit trail opens on what just happened.
	if entries[0].Source != audit.SourceLedger || entries[1].Source != audit.SourceAdmin {
		t.Errorf("order = %s, %s; want the newer ledger entry first",
			entries[0].Source, entries[1].Source)
	}
}

func TestRead_saysWhichRecordAnEntryCameFrom(t *testing.T) {
	reader, pool := readerFor(t)

	seedAdmin(t, pool, noon, "usr_ana", "tool.classified", "cx")
	seedDecision(t, pool, noon.Add(time.Minute), 4, "cx")

	entries, _, _ := reader.Read(t.Context(), audit.Filter{}, 50)

	// Only one of these two records is hash-chained. Calling the merged result
	// "verified" would claim a guarantee half the rows do not have.
	if entries[0].Hash == "" {
		t.Error("a ledger entry carries no seal")
	}
	if entries[1].Hash != "" {
		t.Error("an administrative entry carries a seal it cannot have")
	}
}

func TestRead_namesTheVerdictRatherThanItsNumber(t *testing.T) {
	reader, pool := readerFor(t)

	seedDecision(t, pool, noon, 4, "cx")

	entries, _, _ := reader.Read(t.Context(), audit.Filter{}, 50)
	if len(entries) != 1 || entries[0].Verb != "gate.blocked" {
		t.Errorf("verb = %+v, want gate.blocked — a number is not an audit record", entries)
	}
}

func TestRead_showsOnlyAreasTheCallerReaches(t *testing.T) {
	reader, pool := readerFor(t)

	seedDecision(t, pool, noon, 4, "cx")
	seedDecision(t, pool, noon.Add(time.Minute), 4, "marketing")
	seedAdmin(t, pool, noon.Add(2*time.Minute), "usr_ana", "tool.classified", "marketing")

	entries, _, err := reader.Read(t.Context(), audit.Filter{
		Scopes: []domain.Scope{{Company: "acme", Area: "cx"}},
	}, 50)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	// An audit trail that showed an area somebody cannot otherwise see would
	// be a way around every other check on this platform.
	for _, entry := range entries {
		if entry.Scope.Area != "cx" && entry.Scope.Area != "" {
			t.Errorf("entry from %s reached a caller granted only in cx", entry.Scope.Area)
		}
	}
	if len(entries) != 1 {
		t.Errorf("entries = %d, want only the one in cx", len(entries))
	}
}

func TestRead_narrowsToOneRecordWhenAsked(t *testing.T) {
	reader, pool := readerFor(t)

	seedAdmin(t, pool, noon, "usr_ana", "tool.classified", "cx")
	seedDecision(t, pool, noon.Add(time.Minute), 4, "cx")

	entries, _, err := reader.Read(t.Context(),
		audit.Filter{Sources: []audit.Source{audit.SourceAdmin}}, 50)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 1 || entries[0].Source != audit.SourceAdmin {
		t.Errorf("entries = %+v, want only administrative ones", entries)
	}
}

func TestRead_narrowsToOneActor_byPartOfTheIdentifier(t *testing.T) {
	reader, pool := readerFor(t)

	seedAdmin(t, pool, noon, "usr_ana", "tool.classified", "cx")
	seedAdmin(t, pool, noon.Add(time.Minute), "usr_bruno", "provider.created", "cx")

	entries, _, err := reader.Read(t.Context(), audit.Filter{Actor: "ana"}, 50)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 1 || entries[0].Actor != "usr_ana" {
		t.Errorf("entries = %+v, want only what usr_ana did", entries)
	}
}

func TestRead_boundsTheWindowAtBothEnds(t *testing.T) {
	reader, pool := readerFor(t)

	seedAdmin(t, pool, noon.Add(-2*time.Hour), "usr_ana", "tool.classified", "cx")
	seedAdmin(t, pool, noon, "usr_ana", "provider.created", "cx")
	seedAdmin(t, pool, noon.Add(2*time.Hour), "usr_ana", "budget.set", "cx")

	entries, _, err := reader.Read(t.Context(), audit.Filter{
		Since: noon.Add(-time.Hour), Until: noon.Add(time.Hour),
	}, 50)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 1 || entries[0].Verb != "provider.created" {
		t.Errorf("entries = %+v, want only the one inside the window", entries)
	}
}
