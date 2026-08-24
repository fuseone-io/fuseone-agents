package admin_test

import (
	"context"
	"testing"
	"time"

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
