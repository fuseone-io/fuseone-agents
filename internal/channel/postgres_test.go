package channel_test

import (
	"errors"
	"os"
	"strings"
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

	awaitApproval(t, pool, "run-waiting")

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
		Announcement: channel.Announcement{
			RunID: "run-waiting", Event: channel.EventParked,
			Channel: "acme-slack", Conversation: "C07-ops",
		},
		Ref: "1.1", PostedAt: time.Now(),
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
	if err := store.Reported(t.Context(), pending[0], noon); err != nil {
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

func TestRecordFailure_keepsScopeAndCountsRetries(t *testing.T) {
	store, pool := channelStore(t)
	failure := channel.DeliveryFailure{
		Announcement: channel.Announcement{
			RunID: "run-waiting", Event: channel.EventParked,
			Channel: "acme-slack", Conversation: "C07-ops",
		},
		Code: "slack-team-alerts", Scope: domain.Scope{Company: "acme", Area: "ops"},
		AgentID: "triage", SeenAt: noon,
	}

	if err := store.RecordFailure(t.Context(), failure); err != nil {
		t.Fatalf("first RecordFailure: %v", err)
	}
	failure.SeenAt = noon.Add(time.Minute)
	if err := store.RecordFailure(t.Context(), failure); err != nil {
		t.Fatalf("second RecordFailure: %v", err)
	}

	var company, area, code string
	var scopeWide bool
	var attempts int64
	var first, last time.Time
	err := pool.QueryRow(t.Context(), `
		select company_id, area_id, code, scope_wide, attempts, first_seen, last_seen
		from channel_delivery_failures
		where run_id = 'run-waiting'`).Scan(&company, &area, &code, &scopeWide, &attempts, &first, &last)
	if err != nil {
		t.Fatalf("read failure: %v", err)
	}
	if company != "acme" || area != "ops" || code != channel.MetricOther ||
		scopeWide || attempts != 2 {
		t.Fatalf("failure row = %s/%s %s scopeWide=%v attempts=%d",
			company, area, code, scopeWide, attempts)
	}
	if !first.Equal(noon) || !last.Equal(noon.Add(time.Minute)) {
		t.Fatalf("first=%s last=%s, want retry window", first, last)
	}
}

func TestRuntimeHealth_channelFailuresAreScopedAndBounded(t *testing.T) {
	store, pool := channelStore(t)
	ctx := t.Context()
	ops := domain.Scope{Company: "acme", Area: "ops"}
	finance := domain.Scope{Company: "acme", Area: "finance"}

	recordFailure := func(run, conversation, code string, scope domain.Scope, seen time.Time) {
		t.Helper()
		if err := store.RecordFailure(ctx, channel.DeliveryFailure{
			Announcement: channel.Announcement{
				RunID: domain.RunID(run), Event: channel.EventParked,
				Channel: "acme-slack", Conversation: conversation,
			},
			Code: code, Scope: scope, AgentID: "triage", SeenAt: seen,
		}); err != nil {
			t.Fatalf("RecordFailure %s/%s: %v", run, code, err)
		}
	}

	recordFailure("run-ops-1", "C07-ops", channel.CodeMissingScope, ops, noon)
	recordFailure("run-ops-1", "C07-ops", channel.CodeMissingScope, ops, noon.Add(time.Minute))
	recordFailure("run-ops-2", "C08-ops", "jira-prod.transition_ACME-4417", ops, noon.Add(2*time.Minute))
	if err := store.RecordFailure(ctx, channel.DeliveryFailure{
		Announcement: channel.Announcement{
			RunID: "run-ops-3", Event: channel.EventParked,
		},
		Code: channel.CodeConfigurationReadFailed, Scope: ops,
		AgentID: "triage", ScopeWide: true, SeenAt: noon.Add(3 * time.Minute),
	}); err != nil {
		t.Fatalf("RecordFailure scope-wide: %v", err)
	}
	recordFailure("run-finance", "C09-finance", channel.CodeDeliveryFailed, finance, noon.Add(4*time.Minute))

	health, err := ledger.NewPostgres(pool).RuntimeHealth(ctx, domain.RunFilter{
		Scope: ops, Since: noon.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("RuntimeHealth: %v", err)
	}
	if len(health.ChannelFailures) != 3 {
		t.Fatalf("ChannelFailures = %+v, want three scoped buckets", health.ChannelFailures)
	}

	byCode := map[string]domain.RuntimeChannelFailureBucket{}
	for _, bucket := range health.ChannelFailures {
		byCode[bucket.Code] = bucket
	}
	missing := byCode[channel.CodeMissingScope]
	if missing.Code != channel.CodeMissingScope || missing.Attempts != 2 ||
		missing.Conversations != 1 || missing.ScopeWide || missing.Runs != 1 {
		t.Fatalf("missing scope bucket = %+v", missing)
	}
	if !missing.FirstAt.Equal(noon) || !missing.LastAt.Equal(noon.Add(time.Minute)) {
		t.Fatalf("missing scope window = %s..%s", missing.FirstAt, missing.LastAt)
	}

	other := byCode[channel.MetricOther]
	if other.Code != channel.MetricOther || other.Attempts != 1 ||
		other.Conversations != 1 || other.ScopeWide || other.Runs != 1 {
		t.Fatalf("dynamic code bucket = %+v, want bounded other", other)
	}

	scopeWide := byCode[channel.CodeConfigurationReadFailed]
	if scopeWide.Code != channel.CodeConfigurationReadFailed || scopeWide.Attempts != 1 ||
		scopeWide.Conversations != 0 || !scopeWide.ScopeWide || scopeWide.Runs != 1 {
		t.Fatalf("scope-wide bucket = %+v, want no invented conversation", scopeWide)
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
	if _, err := pool.Exec(t.Context(), `
		truncate run_steps, runs, channel_deliveries, channel_delivery_failures`); err != nil {
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

// awaitApproval stops a run on a person. Named for what it appends: a run also
// stops without asking anybody — a budget, a retry that stopped helping — and
// reading `park` as covering both is how the second of those kept its defect
// while the first was fixed.
func awaitApproval(t *testing.T, pool *pgxpool.Pool, run string) {
	t.Helper()
	appendStep(t, pool, run, domain.StepRunStarted, nil)
	appendStep(t, pool, run, domain.StepApprovalRequested,
		[]byte(`{"tool":"erp.transfer","rule":"financial","reason":"over the ceiling"}`))
}

// parkWithoutAsking is the other way a run stops: nothing to decide, nothing
// pending, and no approval sequence on the projection at all.
func parkWithoutAsking(t *testing.T, pool *pgxpool.Pool, run string) {
	t.Helper()
	appendStep(t, pool, run, domain.StepRunStarted, nil)
	appendStep(t, pool, run, domain.StepParked, []byte(`{"reason":"over the budget"}`))
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
		Announcement: channel.Announcement{
			RunID: "run-alerta", Event: channel.EventParked,
			Channel: "acme-slack", Conversation: "C07-ops",
		},
		Ref: "1786.42", PostedAt: noon,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, ok, err := store.AboutRun(t.Context(), "acme-slack", "C07-ops", "1786.42")
	if err != nil {
		t.Fatalf("AboutRun: %v", err)
	}
	if !ok || got != "run-alerta" {
		t.Errorf("resolved to %q (%v), want run-alerta", got, ok)
	}
}

func TestAboutRun_aMessageSomebodyElsePosted_resolvesToNothing(t *testing.T) {
	store, _ := channelStore(t)

	_, ok, err := store.AboutRun(t.Context(), "acme-slack", "C07-ops", "9999.11")
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
		Announcement: channel.Announcement{
			RunID: "run-outra", Event: channel.EventParked,
			Channel: "acme-slack", Conversation: "C08-finance",
		},
		Ref: "1786.77", PostedAt: noon,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	if _, ok, _ := store.AboutRun(t.Context(), "acme-slack", "C07-ops", "1786.77"); ok {
		t.Error("a reference resolved across conversations")
	}
}

/*
The same conversation id and the same ref on two connections do not cross.

Slack's timestamps and another vendor's message ids are two namespaces, and a
reply in one workspace resolving to a run reported in another would name a run
the replier cannot read, from a channel they can.
*/
func TestAboutRun_theSameRefOnAnotherConnection_isAnotherMessage(t *testing.T) {
	store, _ := channelStore(t)

	if err := store.Record(t.Context(), channel.Delivery{
		Announcement: channel.Announcement{
			RunID: "run-teams", Event: channel.EventParked,
			Channel: "acme-teams", Conversation: "SHARED-ID",
		},
		Ref: "1786.99", PostedAt: noon,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	if _, ok, _ := store.AboutRun(t.Context(), "acme-slack", "SHARED-ID", "1786.99"); ok {
		t.Error("a reference resolved across connections")
	}
}

/*
A run that stops a second time is a second question.

The sentinel that retires a run from the sweep is keyed by the run and the
event, so once a parked run had been announced it could never be announced
again — and an agent that asks for two tools asks for two approvals. The second
one reached no channel and no person, and the run sat parked until somebody
happened to open the console.
*/
func TestUnreported_runParksAtASecondStep_isListedAgain(t *testing.T) {
	store, pool := channelStore(t)

	awaitApproval(t, pool, "run-twice")
	first, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("pending = %+v, want the first park", first)
	}
	if err := store.Reported(t.Context(), first[0], noon); err != nil {
		t.Fatalf("reported: %v", err)
	}

	decideAndParkAgain(t, pool, "run-twice")

	again, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported after the second park: %v", err)
	}
	if len(again) != 1 || again[0].RunID != "run-twice" {
		t.Fatalf("pending = %+v, want the second park announced", again)
	}
	if again[0].AtSeq <= first[0].AtSeq {
		t.Errorf("at seq %d, want the later step (first was %d)", again[0].AtSeq, first[0].AtSeq)
	}
}

// And a run sitting on the same question is asked about once. The sweep runs
// every thirty seconds; a run parked overnight must not be announced two
// thousand times.
func TestUnreported_runStillParkedAtTheSameStep_isReportedOnce(t *testing.T) {
	store, pool := channelStore(t)

	awaitApproval(t, pool, "run-patient")
	pending, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	if err := store.Reported(t.Context(), pending[0], noon); err != nil {
		t.Fatalf("reported: %v", err)
	}

	again, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported after being reported: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("announced the same question twice: %+v", again)
	}
}

/*
And a run that stops without asking anybody stops as often as it needs to.

A budget park and a retry that stopped helping carry no approval, so the
projection has no pending sequence for them at all. Keyed off that alone, both
parks are filed under the same zero and the second one is announced to nobody —
the same defect as a repeated approval, in the shape that has no button.
*/
func TestUnreported_runParksAgainWithoutAsking_isListedAgain(t *testing.T) {
	store, pool := channelStore(t)

	parkWithoutAsking(t, pool, "run-budget")
	first, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("pending = %+v, want the first park", first)
	}
	if err := store.Reported(t.Context(), first[0], noon); err != nil {
		t.Fatalf("reported: %v", err)
	}

	appendStep(t, pool, "run-budget", domain.StepResumed, []byte(`{"by":"usr_ana"}`))
	appendStep(t, pool, "run-budget", domain.StepParked, []byte(`{"reason":"over the budget again"}`))

	again, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported after the second park: %v", err)
	}
	if len(again) != 1 || again[0].RunID != "run-budget" {
		t.Fatalf("pending = %+v, want the second park announced", again)
	}
	if again[0].AtSeq <= first[0].AtSeq {
		t.Errorf("at seq %d, want the later stop (first was %d)", again[0].AtSeq, first[0].AtSeq)
	}
}

// A run sitting on the same stop is asked about once, whether or not anybody
// was asked to decide it.
func TestUnreported_runStillParkedWithoutAsking_isReportedOnce(t *testing.T) {
	store, pool := channelStore(t)

	parkWithoutAsking(t, pool, "run-budget-patient")
	pending, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	if err := store.Reported(t.Context(), pending[0], noon); err != nil {
		t.Fatalf("reported: %v", err)
	}

	again, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported after being reported: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("announced the same stop twice: %+v", again)
	}
}

func decideAndParkAgain(t *testing.T, pool *pgxpool.Pool, run string) {
	t.Helper()
	appendStep(t, pool, run, domain.StepApprovalDecided,
		[]byte(`{"approved":true,"by":"usr_ana"}`))
	appendStep(t, pool, run, domain.StepApprovalRequested,
		[]byte(`{"tool":"erp.pay","rule":"financial","reason":"a second ceiling"}`))
}

/*
A delivery that names no conversation is refused, not stored.

An empty connection and an empty conversation is the shape a run reported
everywhere is filed under. A delivery reaching it — a recipient nobody bound, a
lookup that answered nothing — would retire the run from the sweep, and every
real conversation would lose the announcement silently and for good. The
invariant is held by the table's writer rather than by everyone who calls it.
*/
func TestRecord_aDeliveryNamingNoPlace_isRefusedAndDoesNotRetireTheRun(t *testing.T) {
	store, pool := channelStore(t)

	for _, missing := range []struct {
		what         string
		channel      string
		conversation string
	}{
		// The shape that means "said everywhere". Stored, it retires the run.
		{"neither", "", ""},
		// The half a direct message produces: the connection is known long
		// before the person's account is. Stored, it claims somebody was told.
		{"no conversation", "acme-slack", ""},
		{"no connection", "", "C07-ops"},
	} {
		run := domain.RunID("run-unaddressed-" + strings.ReplaceAll(missing.what, " ", "-"))
		awaitApproval(t, pool, string(run))

		err := store.Record(t.Context(), channel.Delivery{
			Announcement: channel.Announcement{
				RunID: run, Event: channel.EventParked,
				Channel: missing.channel, Conversation: missing.conversation,
			},
			PostedAt: noon,
		})
		if !errors.Is(err, channel.ErrUnaddressed) {
			t.Errorf("%s: Record = %v, want it refused as unaddressed", missing.what, err)
		}

		pending, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
		if err != nil {
			t.Fatalf("unreported: %v", err)
		}
		if !owes(pending, run) {
			t.Errorf("%s: the run is no longer owed an announcement", missing.what)
		}
	}
}

func owes(pending []channel.Report, run domain.RunID) bool {
	for _, r := range pending {
		if r.RunID == run {
			return true
		}
	}
	return false
}

/*
A run awaiting a decision with no pending sequence is announced once, not
forever.

The projection always writes one for an approval request, so this is a row that
arrived some other way — a restore, a migration, a writer older than the column.
It matters because of how SQL compares: `at_seq = null` is never true, so a
sentinel could never match and the sweep would announce the same run every
thirty seconds until somebody noticed the noise. The fallback to the run's own
last step is what makes the comparison possible at all.
*/
func TestUnreported_awaitingApprovalWithNoPendingSequence_isStillRetired(t *testing.T) {
	store, pool := channelStore(t)

	if _, err := pool.Exec(t.Context(), `
		insert into runs (run_id, company_id, area_id, agent_id, version_id,
		                  phase, last_seq, pending_at_seq, started_at, updated_at)
		values ('run-restored', 'acme', 'ops', 'triage', 'v1',
		        'awaiting_approval', 7, null, $1, $1)`, noon); err != nil {
		t.Fatalf("seed a restored row: %v", err)
	}

	pending, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	if len(pending) != 1 || pending[0].AtSeq != 7 {
		t.Fatalf("pending = %+v, want the run at its own last step", pending)
	}
	if err := store.Reported(t.Context(), pending[0], noon); err != nil {
		t.Fatalf("reported: %v", err)
	}

	again, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported after being reported: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("announced again with nothing new to say: %+v", again)
	}
}

/*
Which posted cards still ask a question that has an answer.

Read as state rather than pushed from the decision, so it has to recognise
every way a question stops being open: somebody answered it, the run was
abandoned, or the run stopped again somewhere later — which happens now that a
run parking twice is announced twice, and the first card must stop offering to
answer a step the second one replaced.
*/
func TestStale_cardsWhoseQuestionIsSettled_areListedWithWhatHappened(t *testing.T) {
	store, pool := channelStore(t)

	awaitApproval(t, pool, "run-decided")
	pending, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	if err := store.Record(t.Context(), channel.Delivery{
		Announcement: pending[0].AnnouncementTo(channel.Conversation{
			Channel: "acme-slack", ID: "C07-ops",
		}),
		Ref: "1786.1", PostedAt: noon,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Still waiting: the card is asking a live question.
	open, err := store.Stale(t.Context(), 50)
	if err != nil {
		t.Fatalf("Stale: %v", err)
	}
	if cardFor(open, "run-decided") != nil {
		t.Fatal("a card was called stale while the run was still waiting")
	}

	appendStep(t, pool, "run-decided", domain.StepApprovalDecided,
		[]byte(`{"approved":true,"by":"usr_ana","at_seq":2}`))

	open, err = store.Stale(t.Context(), 50)
	if err != nil {
		t.Fatalf("Stale after the decision: %v", err)
	}
	card := cardFor(open, "run-decided")
	if card == nil {
		t.Fatalf("open = %+v, want the answered card", open)
	}
	if card.Outcome != channel.OutcomeApproved || card.DecidedBy != "usr_ana" {
		t.Errorf("card = %+v, want it to carry who answered and how", card)
	}
	// The facts the card showed, read back from the step it asked about. The
	// projection clears them the moment the run moves on, so a closed card
	// built from it would answer a question the room can no longer read.
	if card.Tool != "erp.transfer" || card.Reason != "over the ceiling" {
		t.Errorf("card = %+v, want the action and reason it was about", card)
	}

	if err := store.Closed(t.Context(), *card, noon); err != nil {
		t.Fatalf("Closed: %v", err)
	}
	open, err = store.Stale(t.Context(), 50)
	if err != nil {
		t.Fatalf("Stale after closing: %v", err)
	}
	if cardFor(open, "run-decided") != nil {
		t.Error("a closed card came back for another sweep")
	}
}

// A run that stopped and was never decided says exactly that. Calling it
// refused would put a decision in somebody's mouth that nobody made.
func TestStale_aRunThatMovedOnUndecided_claimsNoDecision(t *testing.T) {
	store, pool := channelStore(t)

	awaitApproval(t, pool, "run-abandoned")
	pending, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	if err := store.Record(t.Context(), channel.Delivery{
		Announcement: pending[0].AnnouncementTo(channel.Conversation{
			Channel: "acme-slack", ID: "C07-ops",
		}),
		Ref: "1786.2", PostedAt: noon,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	appendStep(t, pool, "run-abandoned", domain.StepAbandoned, []byte(`{"reason":"cancelled"}`))

	open, err := store.Stale(t.Context(), 50)
	if err != nil {
		t.Fatalf("Stale: %v", err)
	}
	card := cardFor(open, "run-abandoned")
	if card == nil {
		t.Fatalf("open = %+v, want the card of a run nobody decided", open)
	}
	if card.Outcome != channel.OutcomeMovedOn || card.DecidedBy != "" {
		t.Errorf("card = %+v, want it to claim no decision", card)
	}
}

/*
The sentinel is not a card, and neither is a row naming no message.

"Said everywhere" is filed as a delivery with no connection and no
conversation; a row with no reference names nothing that could be rewritten.
Routing either to a driver asks it to edit a message that does not exist, on a
connection that is not one.
*/
func TestStale_theSentinelAndRowsNamingNoMessage_areNotCards(t *testing.T) {
	store, pool := channelStore(t)

	awaitApproval(t, pool, "run-sentinel")
	pending, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	if err := store.Reported(t.Context(), pending[0], noon); err != nil {
		t.Fatalf("Reported: %v", err)
	}
	if err := store.Record(t.Context(), channel.Delivery{
		Announcement: pending[0].AnnouncementTo(channel.Conversation{
			Channel: "acme-slack", ID: "C08-quiet",
		}),
		PostedAt: noon,
	}); err != nil {
		t.Fatalf("Record without a ref: %v", err)
	}
	appendStep(t, pool, "run-sentinel", domain.StepAbandoned, []byte(`{"reason":"cancelled"}`))

	open, err := store.Stale(t.Context(), 50)
	if err != nil {
		t.Fatalf("Stale: %v", err)
	}
	for _, c := range open {
		if c.RunID != "run-sentinel" {
			continue
		}
		if c.Conversation == "" || c.Ref == "" {
			t.Errorf("card = %+v, want nothing that names no message", c)
		}
	}
}

func cardFor(open []channel.Card, run domain.RunID) *channel.Card {
	for i, c := range open {
		if c.RunID == run {
			return &open[i]
		}
	}
	return nil
}

/*
A run waiting again is not waiting on the old question.

A run stops as many times as it asks, and each stop is announced now. The card
from the first stop is still on screen offering to answer a step the second one
replaced — and the run is once more awaiting a decision, so anything that
looked only at the phase would leave it open for ever.
*/
func TestStale_aRunWaitingOnALaterStep_closesTheEarlierCard(t *testing.T) {
	store, pool := channelStore(t)

	awaitApproval(t, pool, "run-again")
	first, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	if err := store.Record(t.Context(), channel.Delivery{
		Announcement: first[0].AnnouncementTo(channel.Conversation{
			Channel: "acme-slack", ID: "C07-ops",
		}),
		Ref: "1786.3", PostedAt: noon,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	decideAndParkAgain(t, pool, "run-again")

	open, err := store.Stale(t.Context(), 50)
	if err != nil {
		t.Fatalf("Stale: %v", err)
	}
	card := cardFor(open, "run-again")
	if card == nil {
		t.Fatalf("open = %+v, want the card of the question that was replaced", open)
	}
	if card.AtSeq != first[0].AtSeq {
		t.Errorf("card at seq %d, want the earlier question %d", card.AtSeq, first[0].AtSeq)
	}
}

/*
Whether a stop is a question somebody can answer.

Both kinds of stop carry a sequence now, so the number cannot tell them apart —
and read as "a park with a sequence is a pending approval", every budget park
draws two buttons whose only possible answer is a conflict and messages every
approver about a decision nobody can make. The phase is what knows, and this is
where it is read.
*/
func TestUnreported_saysWhetherTheStopIsAQuestion(t *testing.T) {
	store, pool := channelStore(t)

	awaitApproval(t, pool, "run-asking")
	parkWithoutAsking(t, pool, "run-just-stopped")

	pending, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	for _, want := range []struct {
		run    domain.RunID
		asking bool
	}{
		{"run-asking", true},
		{"run-just-stopped", false},
	} {
		report := reportFor(pending, want.run)
		if report == nil {
			t.Fatalf("pending = %+v, want %s", pending, want.run)
		}
		if report.AwaitingDecision != want.asking {
			t.Errorf("%s: awaiting a decision = %v, want %v",
				want.run, report.AwaitingDecision, want.asking)
		}
	}
}

func reportFor(pending []channel.Report, run domain.RunID) *channel.Report {
	for i, r := range pending {
		if r.RunID == run {
			return &pending[i]
		}
	}
	return nil
}

/*
A stop that asked nothing is not a card.

A run parked by its budget carries a step like any other stop now, and its
message has no buttons on it — there is nothing to close. Swept up with the
approvals, a perfectly good "stopped: over budget" is rewritten into an answer
to a question nobody asked, in a room where people are reading it.
*/
func TestStale_aStopThatAskedNothing_isNotACard(t *testing.T) {
	store, pool := channelStore(t)

	parkWithoutAsking(t, pool, "run-budget-card")
	pending, err := store.Unreported(t.Context(), noon.Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	if err := store.Record(t.Context(), channel.Delivery{
		Announcement: pending[0].AnnouncementTo(channel.Conversation{
			Channel: "acme-slack", ID: "C07-ops",
		}),
		Ref: "1786.9", PostedAt: noon,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	appendStep(t, pool, "run-budget-card", domain.StepResumed, []byte(`{"by":"usr_ana"}`))

	open, err := store.Stale(t.Context(), 50)
	if err != nil {
		t.Fatalf("Stale: %v", err)
	}
	if cardFor(open, "run-budget-card") != nil {
		t.Error("a stop that asked nothing was swept up as an approval card")
	}
}
