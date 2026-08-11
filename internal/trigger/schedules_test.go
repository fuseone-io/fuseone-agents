package trigger_test

import (
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/trigger"
)

// The table only remembers when to look. Everything about firing once is the
// idempotency key's job — but a schedule that never becomes due, or one that
// keeps firing after its version stopped declaring it, fails just as loudly.

func schedulesFor(t *testing.T) *trigger.PostgresSchedules {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping the schedule suite")
	}

	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `truncate trigger_schedules`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return trigger.NewPostgresSchedules(pool)
}

func TestSync_declaredSchedule_becomesDueAtItsNextMoment(t *testing.T) {
	rows := schedulesFor(t)

	if err := rows.Sync(t.Context(), "triage", []string{"*/15 * * * *"}, noon); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Not due yet at the moment it was declared.
	if due, err := rows.Due(t.Context(), noon); err != nil || len(due) != 0 {
		t.Fatalf("due at declaration = %v (%v), want none", due, err)
	}
	due, err := rows.Due(t.Context(), noon.Add(16*time.Minute))
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(due) != 1 || !due[0].At.Equal(noon.Add(15*time.Minute)) {
		t.Errorf("due = %+v, want the 15-minute mark", due)
	}
}

func TestSync_republishing_keepsTheMomentItWasAlreadyWaitingFor(t *testing.T) {
	rows := schedulesFor(t)

	if err := rows.Sync(t.Context(), "triage", []string{"0 * * * *"}, noon); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// Publishing again ten minutes later must not push the moment out. An
	// agent published every few minutes would otherwise never run at all.
	if err := rows.Sync(t.Context(), "triage", []string{"0 * * * *"}, noon.Add(10*time.Minute)); err != nil {
		t.Fatalf("Sync again: %v", err)
	}

	due, err := rows.Due(t.Context(), noon.Add(time.Hour))
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(due) != 1 || !due[0].At.Equal(noon.Add(time.Hour)) {
		t.Errorf("due = %+v, want the moment it was already waiting for", due)
	}
}

func TestSync_scheduleNoLongerDeclared_stopsFiring(t *testing.T) {
	rows := schedulesFor(t)

	if err := rows.Sync(t.Context(), "triage", []string{"0 * * * *"}, noon); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// The new version dropped the schedule. It has to stop, or the agent keeps
	// running on a rule nobody can find in its definition.
	if err := rows.Sync(t.Context(), "triage", nil, noon); err != nil {
		t.Fatalf("Sync without schedules: %v", err)
	}

	due, err := rows.Due(t.Context(), noon.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(due) != 0 {
		t.Errorf("due = %+v, want nothing after the schedule was withdrawn", due)
	}
}

func TestSync_unparseableSchedule_isRefusedBeforeItReachesTheTable(t *testing.T) {
	rows := schedulesFor(t)

	// A schedule that cannot be parsed cannot be advanced either, so a row
	// holding one would read as due on every tick forever.
	if err := rows.Sync(t.Context(), "triage", []string{"every other tuesday"}, noon); err == nil {
		t.Fatal("an unparseable schedule was stored")
	}
	if due, _ := rows.Due(t.Context(), noon.Add(24*time.Hour)); len(due) != 0 {
		t.Errorf("due = %+v, want nothing stored", due)
	}
}

func TestAdvance_movesTheMomentForward(t *testing.T) {
	rows := schedulesFor(t)

	if err := rows.Sync(t.Context(), "triage", []string{"0 * * * *"}, noon); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if err := rows.Advance(t.Context(), "triage", "0 * * * *", noon.Add(3*time.Hour)); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	if due, _ := rows.Due(t.Context(), noon.Add(2*time.Hour)); len(due) != 0 {
		t.Errorf("due = %+v, want nothing before the advanced moment", due)
	}
	if due, _ := rows.Due(t.Context(), noon.Add(4*time.Hour)); len(due) != 1 {
		t.Errorf("due = %+v, want the advanced moment", due)
	}
}
