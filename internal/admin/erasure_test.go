package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

func erasuresFor(t *testing.T, now func() time.Time) (*admin.Erasures, *ledger.Content) {
	t.Helper()
	pool := openPool(t)
	if _, err := pool.Exec(context.Background(),
		`delete from run_content;
		 delete from channel_inbox;
		 delete from channel_deliveries;
		 delete from channel_delivery_failures;
		 delete from memory_assertion_events;
		 delete from memory_suggestions;
		 delete from memory_assertions;
		 delete from runs where run_id like 'run-retention-%';
		 delete from settings where kind = 'retention'`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	key := make([]byte, 32)
	v, _ := vault.New(key, "test")
	store := settings.NewStore(pool, v)
	content := ledger.NewContent(pool)
	return admin.NewErasures(pool, content, admin.NewRetention(pool, store)).WithClock(now), content
}

func TestSweep_onTheDefaultWindow_takesNothingRecent(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	ctx := context.Background()

	ref, err := content.Put(ctx, "run-1", 1, []byte(`{"email":"ana@exemplo.com.br"}`))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	erased, err := erasures.Sweep(ctx)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	// Five years is the default, so an installation that has never been
	// configured erases nothing on its first sweep — which is the only safe
	// behaviour for the one job that destroys data.
	if erased != 0 {
		t.Fatalf("erased %d, want none", erased)
	}
	if _, err := content.Get(ctx, ref); err != nil {
		t.Errorf("Get = %v, want it kept", err)
	}
}

func TestSweep_pastTheWindow_erasesAndRecordsIt(t *testing.T) {
	// Six years on, with the default window: everything stored today is past
	// it, and the sweep is what an installation relies on to meet its own
	// retention promise without anybody remembering to.
	future := func() time.Time { return time.Now().Add(6 * 365 * 24 * time.Hour) }
	erasures, content := erasuresFor(t, future)
	ctx := context.Background()

	ref, _ := content.Put(ctx, "run-1", 1, []byte("velho"))
	erased, err := erasures.Sweep(ctx)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if erased != 1 {
		t.Fatalf("erased %d, want the aged object", erased)
	}
	if _, err := content.Get(ctx, ref); err == nil {
		t.Error("the content survived its retention window")
	}

	// An erasure nobody can account for is indistinguishable from data loss.
	action, _ := lastTrailDetail(t, openPool(t), "content")
	if action != "content.expired" {
		t.Errorf("action = %q", action)
	}
}

func TestSweep_pastTheWindowDeletesChannelOperationalRecords(t *testing.T) {
	now := time.Date(2032, 8, 23, 12, 0, 0, 0, time.UTC)
	erasures, _ := erasuresFor(t, func() time.Time { return now })
	pool := openPool(t)
	ctx := context.Background()
	old, recent := now.Add(-6*365*24*time.Hour), now.Add(-time.Hour)

	if _, err := pool.Exec(ctx, `
		insert into channel_inbox (
			channel, conversation, event_id, message, payload, digest, status, answered_at, at)
		values ('slack', 'C-old', 'E-old', 'M-old', 'old', 'digest', 'refused', $1, $1)`,
		old); err != nil {
		t.Fatalf("seed inbox: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into runs (
			run_id, company_id, area_id, agent_id, version_id,
			phase, last_seq, started_at, ended_at, updated_at)
		values (
			'run-retention-finished', 'acme', 'ops', 'triage', 'v1',
			'finished', 3, $1, $2, $2)`,
		old, recent); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into channel_inbox (
			channel, conversation, event_id, message, payload, digest,
			status, run_id, answer_due, at)
		values
			('slack', 'C-owed-refusal', 'E-owed-refusal', 'M-owed-refusal',
			 'owed', 'digest-owed-refusal', 'refused', '', false, $1),
			('slack', 'C-owed-final', 'E-owed-final', 'M-owed-final',
			 'owed', 'digest-owed-final', 'opened', 'run-retention-finished', true, $1)`,
		old); err != nil {
		t.Fatalf("seed owed inbox: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into channel_deliveries (
			run_id, event, channel, conversation, ref, posted_at)
		values ('run-old', 'parked', 'slack', 'C-old', '1.1', $1)`,
		old); err != nil {
		t.Fatalf("seed delivery: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into channel_delivery_failures (
			run_id, event, channel, conversation, scope_wide, code,
			company_id, area_id, agent_id, attempts, first_seen, last_seen)
		values
			('run-old', 'parked', 'slack', 'C-old', false, 'channel_missing_scope',
			 'acme', 'ops', 'triage', 1, $1, $1),
			('run-current', 'parked', 'slack', 'C-current', false, 'channel_missing_scope',
			 'acme', 'ops', 'triage', 1, $1, $2)`,
		old, recent); err != nil {
		t.Fatalf("seed failures: %v", err)
	}

	erased, err := erasures.Sweep(ctx)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if erased != 3 {
		t.Fatalf("erased %d, want three expired channel records", erased)
	}

	var inbox, deliveries, failures, owedRefusals, owedFinalAnswers int
	if err := pool.QueryRow(ctx, `select count(*) from channel_inbox`).Scan(&inbox); err != nil {
		t.Fatalf("count inbox: %v", err)
	}
	if err := pool.QueryRow(ctx, `select count(*) from channel_deliveries`).Scan(&deliveries); err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	if err := pool.QueryRow(ctx, `select count(*) from channel_delivery_failures`).Scan(&failures); err != nil {
		t.Fatalf("count failures: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		select count(*) from channel_inbox
		where status = 'refused' and answered_at is null`).Scan(&owedRefusals); err != nil {
		t.Fatalf("count owed refusals: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		select count(*) from channel_inbox
		where status = 'opened' and answer_due and answered_at is null and run_id <> ''
		  and exists (
		    select 1 from runs
		    where runs.run_id = channel_inbox.run_id
		      and runs.phase = 'finished'
		      and not runs.simulated
		  )`).Scan(&owedFinalAnswers); err != nil {
		t.Fatalf("count owed final answers: %v", err)
	}
	if inbox != 2 || deliveries != 0 || failures != 1 ||
		owedRefusals != 1 || owedFinalAnswers != 1 {
		t.Fatalf("remaining inbox=%d deliveries=%d failures=%d owedRefusals=%d owedFinalAnswers=%d, want only current channel debts kept",
			inbox, deliveries, failures, owedRefusals, owedFinalAnswers)
	}
}

func TestSweep_pastTheWindowDeletesMemoryOperationalRecords(t *testing.T) {
	now := time.Date(2032, 8, 23, 12, 0, 0, 0, time.UTC)
	erasures, _ := erasuresFor(t, func() time.Time { return now })
	pool := openPool(t)
	ctx := context.Background()
	old, recent := now.Add(-6*365*24*time.Hour), now.Add(-time.Hour)

	seedMemoryAssertion(t, pool, "mem-old", old, nil)
	seedMemoryAssertion(t, pool, "mem-expired", recent, &old)
	seedMemoryAssertion(t, pool, "mem-recent", recent, nil)
	seedMemorySuggestion(t, pool, "mems-old", "mem-suggest-old", old, nil)
	seedMemorySuggestion(t, pool, "mems-expired", "mem-suggest-expired", recent, &old)
	seedMemorySuggestion(t, pool, "mems-recent", "mem-suggest-recent", recent, nil)
	if _, err := pool.Exec(ctx, `
		insert into memory_assertion_events (
			assertion_id, action, company_id, area_id, principal_id, reason, detail, at)
		values
			('mem-old', 'asserted', 'acme', 'ops', 'usr_ana', 'old', '{}', $1),
			('mem-recent', 'asserted', 'acme', 'ops', 'usr_ana', 'recent', '{}', $2)`,
		old, recent); err != nil {
		t.Fatalf("seed memory events: %v", err)
	}

	erased, err := erasures.Sweep(ctx)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if erased != 5 {
		t.Fatalf("erased %d, want old event, two assertions and two suggestions", erased)
	}

	var assertions, suggestions, events int
	if err := pool.QueryRow(ctx, `select count(*) from memory_assertions`).Scan(&assertions); err != nil {
		t.Fatalf("count memory assertions: %v", err)
	}
	if err := pool.QueryRow(ctx, `select count(*) from memory_suggestions`).Scan(&suggestions); err != nil {
		t.Fatalf("count memory suggestions: %v", err)
	}
	if err := pool.QueryRow(ctx, `select count(*) from memory_assertion_events`).Scan(&events); err != nil {
		t.Fatalf("count memory events: %v", err)
	}
	if assertions != 1 || suggestions != 1 || events != 1 {
		t.Fatalf("remaining assertions=%d suggestions=%d events=%d, want only recent memory", assertions, suggestions, events)
	}
}

func TestForSubject_erasesTheRunsItWasGivenAndNoOthers(t *testing.T) {
	erasures, content := erasuresFor(t, time.Now)
	ctx := context.Background()

	mine, _ := content.Put(ctx, "run-mine", 1, []byte("mine"))
	theirs, _ := content.Put(ctx, "run-theirs", 1, []byte("theirs"))

	if _, err := erasures.ForSubject(ctx, "usr_ana", domain.Scope{},
		[]domain.RunID{"run-mine"}, "titular pediu"); err != nil {
		t.Fatalf("ForSubject: %v", err)
	}

	if _, err := content.Get(ctx, mine); err == nil {
		t.Error("the subject's content survived the erasure")
	}
	// Reaching a neighbour's data is the same failure as not erasing at all,
	// in the other direction.
	if _, err := content.Get(ctx, theirs); err != nil {
		t.Errorf("Get(theirs) = %v, want it untouched", err)
	}
}

func TestForSubject_marksMemoryFromErasedRunsUnavailable(t *testing.T) {
	now := time.Date(2032, 8, 23, 12, 0, 0, 0, time.UTC)
	erasures, content := erasuresFor(t, func() time.Time { return now })
	pool := openPool(t)
	ctx := context.Background()

	_, _ = content.Put(ctx, "run-mine", 1, []byte("mine"))
	_, _ = content.Put(ctx, "run-theirs", 1, []byte("theirs"))
	seedMemoryAssertionForRun(t, pool, "mem-mine", "run-mine")
	seedMemoryAssertionForRun(t, pool, "mem-theirs", "run-theirs")
	seedMemorySuggestionForRun(t, pool, "mems-mine", "mem-suggest-mine", "run-mine")
	seedMemorySuggestionForRun(t, pool, "mems-theirs", "mem-suggest-theirs", "run-theirs")

	if _, err := erasures.ForSubject(ctx, "usr_ana", domain.Scope{},
		[]domain.RunID{"run-mine"}, "titular pediu"); err != nil {
		t.Fatalf("ForSubject: %v", err)
	}

	statusMine := memoryStatus(t, pool, "mem-mine")
	statusTheirs := memoryStatus(t, pool, "mem-theirs")
	suggestionMine := memorySuggestionStatus(t, pool, "mems-mine")
	suggestionTheirs := memorySuggestionStatus(t, pool, "mems-theirs")
	if statusMine != "source_erased" || statusTheirs != "active" ||
		suggestionMine != "source_erased" || suggestionTheirs != "pending" {
		t.Fatalf("statuses mine=%s theirs=%s suggestionMine=%s suggestionTheirs=%s, want erased rows marked and neighbours kept",
			statusMine, statusTheirs, suggestionMine, suggestionTheirs)
	}
	if got := memoryEventCount(t, pool, "source_erased"); got != 2 {
		t.Fatalf("source_erased events = %d, want assertion and suggestion events", got)
	}
}

/*
A memory somebody disabled still loses its source when the run goes.

The sweep only reached active rows, so a disabled memory whose run was erased
stayed merely disabled — indistinguishable from one somebody turned off last
Tuesday and can turn back on. Reactivation would then be a way around retention:
the status flips, the row becomes readable again, and the evidence points at a
run that no longer exists.

Disabled is not a terminal state and source_erased is. Which one a row is in has
to be decided by what happened to it, not by whether it happened to be readable
at the moment the erasure ran.
*/
func TestForSubject_marksDisabledMemoryFromErasedRunsUnavailable(t *testing.T) {
	now := time.Date(2032, 8, 23, 12, 0, 0, 0, time.UTC)
	erasures, content := erasuresFor(t, func() time.Time { return now })
	pool := openPool(t)
	ctx := context.Background()

	_, _ = content.Put(ctx, "run-mine", 1, []byte("mine"))
	seedMemoryAssertionForRun(t, pool, "mem-off", "run-mine")
	seedMemoryAssertionForRun(t, pool, "mem-gone", "run-mine")
	for _, id := range []string{"mem-off", "mem-gone"} {
		if _, err := pool.Exec(ctx,
			`update memory_assertions set status = 'disabled' where assertion_id = $1`, id); err != nil {
			t.Fatalf("disable %s: %v", id, err)
		}
	}
	// One of them is disabled and keeps its run, so the test also says what the
	// erasure must not do: turn every disabled row terminal.
	if _, err := pool.Exec(ctx, `
		update memory_assertions
		set evidence = '[{"run_id":"run-kept","artifact":"final_answer","digest":"sha256:answer"}]'
		where assertion_id = 'mem-off'`); err != nil {
		t.Fatalf("repoint mem-off: %v", err)
	}

	if _, err := erasures.ForSubject(ctx, "usr_ana", domain.Scope{},
		[]domain.RunID{"run-mine"}, "titular pediu"); err != nil {
		t.Fatalf("ForSubject: %v", err)
	}

	if got := memoryStatus(t, pool, "mem-gone"); got != "source_erased" {
		t.Errorf("status = %s, want the disabled memory to lose its source too", got)
	}
	if got := memoryStatus(t, pool, "mem-off"); got != "disabled" {
		t.Errorf("status = %s, want a disabled memory whose run remains left alone", got)
	}
	if got := memoryEventCount(t, pool, "source_erased"); got != 1 {
		t.Errorf("source_erased events = %d, want one for the row that lost its run", got)
	}
}

func seedMemoryAssertion(t *testing.T, pool interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, id string, updated time.Time, expires *time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		insert into memory_assertions (
			assertion_id, company_id, area_id, agent_id, kind, subject,
			signature, claim, evidence, observations, confirmed, labels,
			status, expires_at, created_by, created_at, updated_by, updated_at)
		values (
			$1, 'acme', 'ops', 'triage', 'incident', $1,
			$1, 'known incident behaviour', '[]', 1, 1, '{}',
			'active', $2, 'usr_ana', $3, 'usr_ana', $3)`,
		id, expires, updated); err != nil {
		t.Fatalf("seed memory assertion %s: %v", id, err)
	}
}

func seedMemoryAssertionForRun(t *testing.T, pool interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, id string, run domain.RunID) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		insert into memory_assertions (
			assertion_id, company_id, area_id, agent_id, kind, subject,
			signature, claim, evidence, observations, confirmed, labels,
			status, created_by, created_at, updated_by, updated_at)
		values (
			$1, 'acme', 'ops', 'triage', 'incident', $1,
			$1, 'known incident behaviour', $2::jsonb, 1, 1, '{}',
			'active', 'usr_ana', now(), 'usr_ana', now())`,
		id, `[{"run_id":"`+string(run)+`","artifact":"final_answer","digest":"sha256:answer"}]`); err != nil {
		t.Fatalf("seed memory assertion %s: %v", id, err)
	}
}

func seedMemorySuggestion(t *testing.T, pool interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, id string, assertionID string, updated time.Time, expires *time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		insert into memory_suggestions (
			suggestion_id, assertion_id, company_id, area_id, agent_id,
			kind, subject, signature, claim, evidence, observations, labels,
			status, expires_at, created_by, created_at, updated_by, updated_at)
		values (
			$1, $2, 'acme', 'ops', 'triage',
			'incident', $2, $2, 'suggested incident behaviour', '[]', 1, '{}',
			'pending', $3, 'agent:triage', $4, 'agent:triage', $4)`,
		id, assertionID, expires, updated); err != nil {
		t.Fatalf("seed memory suggestion %s: %v", id, err)
	}
}

func seedMemorySuggestionForRun(t *testing.T, pool interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, id string, assertionID string, run domain.RunID) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		insert into memory_suggestions (
			suggestion_id, assertion_id, company_id, area_id, agent_id,
			kind, subject, signature, claim, evidence, observations, labels,
			status, created_by, created_at, updated_by, updated_at)
		values (
			$1, $2, 'acme', 'ops', 'triage',
			'incident', $2, $2, 'suggested incident behaviour', $3::jsonb, 1, '{}',
			'pending', 'agent:triage', now(), 'agent:triage', now())`,
		id, assertionID, `[{"run_id":"`+string(run)+`","artifact":"memory_suggestion","digest":"sha256:answer"}]`); err != nil {
		t.Fatalf("seed memory suggestion %s: %v", id, err)
	}
}

func memoryStatus(t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id string) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(context.Background(),
		`select status from memory_assertions where assertion_id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("memory status %s: %v", id, err)
	}
	return status
}

func memorySuggestionStatus(t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id string) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(context.Background(),
		`select status from memory_suggestions where suggestion_id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("memory suggestion status %s: %v", id, err)
	}
	return status
}

func memoryEventCount(t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, action string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		`select count(*) from memory_assertion_events where action = $1`, action).Scan(&count); err != nil {
		t.Fatalf("memory event count: %v", err)
	}
	return count
}
