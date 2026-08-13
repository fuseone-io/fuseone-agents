package channel_test

import (
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

/*
Which runs are worth announcing, asked of a real projection.

The question is "what has not been reported", not "what changed recently". A
window would drop the run that parked while the process was away — and that run
is precisely the one somebody is waiting on.
*/
func TestUnreported_runIsWaitingOnSomebody_isListedUntilItIsReported(t *testing.T) {
	store, pool := channelStore(t)

	park(t, pool, "run-waiting")

	pending, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	if len(pending) != 1 || pending[0].RunID != "run-waiting" {
		t.Fatalf("pending = %+v, want the parked run", pending)
	}
	if pending[0].Event != channel.EventParked {
		t.Errorf("event = %q, want parked", pending[0].Event)
	}

	if err := store.Record(t.Context(), channel.Delivery{
		RunID: "run-waiting", Event: channel.EventParked,
		Conversation: "C07-ops", Ref: "1.1", PostedAt: time.Now(),
	}); err != nil {
		t.Fatalf("record: %v", err)
	}

	pending, err = store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported after recording: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("still pending after being reported: %+v", pending)
	}
}

// A rehearsal is not something to wake anybody for, and an approval request
// for one would teach people to ignore the channel.
func TestUnreported_simulatedRun_isNotAnnounced(t *testing.T) {
	store, pool := channelStore(t)

	simulate(t, pool, "run-rehearsal")

	pending, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	for _, p := range pending {
		if p.RunID == "run-rehearsal" {
			t.Fatal("a simulated run was queued for announcement")
		}
	}
}

func TestUnreported_runIsStillWorking_saysNothing(t *testing.T) {
	store, pool := channelStore(t)

	appendStep(t, pool, "run-busy", domain.StepRunStarted, nil)

	pending, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("announced a run that is still working: %+v", pending)
	}
}

func channelStore(t *testing.T) (*channel.Postgres, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; the projection is a Postgres fact")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `truncate run_steps, runs, channel_deliveries`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return channel.NewPostgres(pool), pool
}

func appendStep(t *testing.T, pool *pgxpool.Pool, run string, kind domain.StepKind, payload []byte) {
	t.Helper()
	store := ledger.NewPostgres(pool)
	if _, err := store.Append(t.Context(), domain.Step{
		RunID: domain.RunID(run), Kind: kind, At: time.Now(),
		Scope:   domain.Scope{Company: "acme", Area: "ops"},
		AgentID: "triage", VersionID: "v1", Payload: payload,
	}); err != nil {
		t.Fatalf("append %s: %v", kind, err)
	}
}

func park(t *testing.T, pool *pgxpool.Pool, run string) {
	t.Helper()
	appendStep(t, pool, run, domain.StepRunStarted, nil)
	appendStep(t, pool, run, domain.StepApprovalRequested,
		[]byte(`{"tool":"erp.transfer","rule":"financial","reason":"over the ceiling"}`))
}

func simulate(t *testing.T, pool *pgxpool.Pool, run string) {
	t.Helper()
	appendStep(t, pool, run, domain.StepRunStarted, []byte(`{"simulated":true,"simulation":"sim-1"}`))
	appendStep(t, pool, run, domain.StepApprovalRequested,
		[]byte(`{"tool":"erp.transfer","rule":"financial"}`))
}
