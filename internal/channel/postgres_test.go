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

	/*
		One conversation heard, and the run is still not reported.

		A run is announced to every conversation that speaks for its scope, and
		this projection knows none of them — so a delivery cannot be the thing
		that clears it. Reading one row as "done" is what left a conversation the
		bot had been removed from never retried, silently, which is the failure
		the sweep exists to prevent.
	*/
	pending, err = store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported after one conversation heard: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("dropped after one conversation of several: %+v", pending)
	}

	// Said everywhere, recorded by the one component that knows what
	// everywhere means.
	if err := store.Reported(t.Context(), "run-waiting", channel.EventParked, noon); err != nil {
		t.Fatalf("reported: %v", err)
	}

	pending, err = store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported after being reported: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("still pending after every conversation heard: %+v", pending)
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

/*
Which run the platform posted a message about.

NT-005 §2.1's boundary of resolution, and it already lived in this table: the
platform resolves references to what it put there. A thread somebody replies to
is resolvable exactly when this installation posted the message that started
it.

Anything else — another bot's alert, "that problem from yesterday" — does not
resolve and must not pretend to. It becomes an ask with no subject, tainted,
and the Gate treats it as what it is. An agent that needs a specific alert can
go and search for one, which is a tool call somebody can audit rather than a
guess the edge made silently.
*/
func TestAboutRun_aMessageThePlatformPosted_resolvesToItsRun(t *testing.T) {
	store, _ := channelStore(t)

	if err := store.Record(t.Context(), channel.Delivery{
		RunID: "run-alerta", Event: channel.EventParked,
		Conversation: "C07-ops", Ref: "1786.42", PostedAt: noon,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, ok, err := store.AboutRun(t.Context(), "C07-ops", "1786.42")
	if err != nil {
		t.Fatalf("AboutRun: %v", err)
	}
	if !ok || got != "run-alerta" {
		t.Errorf("resolved to %q (%v), want run-alerta", got, ok)
	}
}

func TestAboutRun_aMessageSomebodyElsePosted_resolvesToNothing(t *testing.T) {
	store, _ := channelStore(t)

	_, ok, err := store.AboutRun(t.Context(), "C07-ops", "9999.11")
	if err != nil {
		t.Fatalf("AboutRun: %v", err)
	}
	if ok {
		t.Error("a message this installation never posted resolved to a run")
	}
}

// The same message id in another conversation is another message. Resolving
// across conversations would let a reply in one channel name a run reported in
// a channel the replier cannot see.
func TestAboutRun_theSameRefInAnotherConversation_isAnotherMessage(t *testing.T) {
	store, _ := channelStore(t)

	if err := store.Record(t.Context(), channel.Delivery{
		RunID: "run-outra", Event: channel.EventParked,
		Conversation: "C08-finance", Ref: "1786.77", PostedAt: noon,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	if _, ok, _ := store.AboutRun(t.Context(), "C07-ops", "1786.77"); ok {
		t.Error("a reference resolved across conversations")
	}
}
