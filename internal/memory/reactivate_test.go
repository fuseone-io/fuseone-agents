package memory_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/memory"
)

type reactivateStore interface {
	mergeStore
	Disable(context.Context, string, domain.Scope, domain.UserID, string, time.Time) error
	Reactivate(context.Context, *memory.Resolver, memory.ReactivateInput) (domain.MemoryAssertion, error)
	List(context.Context, memory.Filter) ([]domain.MemoryAssertion, error)
}

/*
Reactivating is a new decision, and it is made against the ledger.

Turning the status back would be a way around retention: the row becomes
readable again on the strength of citations nobody looked at, and the run they
name may have been erased in the meantime. So the evidence is proved again
before anything is written, and what comes back is a memory the platform can
still show the source of.

What is preserved is everything the reactivation is not about — the identity,
the claim, the counters, who created it and when, and the citations themselves.
Rewriting those here would make this a second hydration with a person's name on
it.
*/
func TestReactivate_disabledMemoryStillProvable_becomesReadableAgain(t *testing.T) {
	t.Parallel()
	expectReactivationRestoresTheMemory(t, context.Background(), memory.NewMemory())
}

func TestPostgresReactivate_disabledMemoryStillProvable_becomesReadableAgain(t *testing.T) {
	ctx, store := postgresStore(t)
	expectReactivationRestoresTheMemory(t, ctx, store)
}

func expectReactivationRestoresTheMemory(t *testing.T, ctx context.Context, store reactivateStore) {
	t.Helper()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	later := now.Add(48 * time.Hour)
	run, disabled := disabledMemory(t, ctx, store, now)

	back, err := store.Reactivate(ctx, run.resolver(), memory.ReactivateInput{
		ID: disabled.ID, Scope: run.scope, By: "usr_bruno",
		Reason: "the incident came back", Now: later,
	})
	if err != nil {
		t.Fatalf("Reactivate: %v", err)
	}

	if back.Status != domain.MemoryActive {
		t.Errorf("status = %s, want it readable again", back.Status)
	}
	// The expiry is the new decision's, not the old one's: a memory brought
	// back has just been vouched for, and inheriting a vencimento from the day
	// it was switched off would make it disappear again for no reason.
	if back.ExpiresAt == nil || !back.ExpiresAt.Equal(later.Add(memory.DefaultMemoryTTL)) {
		t.Errorf("expiry = %v, want thirty days from this decision", back.ExpiresAt)
	}
	if back.UpdatedBy != "usr_bruno" || !back.UpdatedAt.Equal(later) {
		t.Errorf("updated by %s at %s, want the person who decided it", back.UpdatedBy, back.UpdatedAt)
	}
	if back.CreatedBy != disabled.CreatedBy || !back.CreatedAt.Equal(disabled.CreatedAt) {
		t.Errorf("creation stamp moved to %s/%s, want the original author kept",
			back.CreatedBy, back.CreatedAt)
	}
	if back.ID != disabled.ID || back.Claim != disabled.Claim ||
		back.Observations != disabled.Observations || back.Confirmed != disabled.Confirmed {
		t.Errorf("reactivation changed what the memory says: %+v", back)
	}
	if !slices.EqualFunc(back.Evidence, disabled.Evidence,
		func(a, b domain.MemoryEvidence) bool { return a.Key() == b.Key() }) {
		t.Errorf("evidence = %+v, want the citations left as they were", back.Evidence)
	}

	found, err := store.Find(ctx, domain.MemoryQuery{
		Scope: run.scope, AgentID: "triage", Now: later,
	})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("find returned %d, want the memory recallable again", len(found))
	}
}

// Only disabled comes back. Active is not a thing to reactivate, and
// source_erased is the platform having lost the proof — a status a decision
// cannot undo, which is the whole reason it is separate from disabled.
func TestReactivate_anythingButDisabled_isRefused(t *testing.T) {
	t.Parallel()
	expectOnlyDisabledReactivates(t, context.Background(), memory.NewMemory())
}

func TestPostgresReactivate_anythingButDisabled_isRefused(t *testing.T) {
	ctx, store := postgresStore(t)
	expectOnlyDisabledReactivates(t, ctx, store)
}

func expectOnlyDisabledReactivates(t *testing.T, ctx context.Context, store reactivateStore) {
	t.Helper()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	t.Run("active", func(t *testing.T) {
		run, cited := finished(t, "run-active")
		resolved, err := run.resolver().Resolve(ctx, run.scope, []domain.MemoryEvidence{cited})
		if err != nil {
			t.Fatalf("resolve for the fixture: %v", err)
		}
		stored, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
			a.Scope, a.Subject = run.scope, "still on"
			a.Evidence, a.Labels = resolved, resolved[0].Labels
		}), "usr_ana", "reviewed", now)
		if err != nil {
			t.Fatalf("Assert: %v", err)
		}
		_, err = store.Reactivate(ctx, run.resolver(), memory.ReactivateInput{
			ID: stored.ID, Scope: run.scope, By: "usr_bruno", Reason: "again", Now: now,
		})
		if !errors.Is(err, memory.ErrMemoryTerminal) {
			t.Fatalf("Reactivate = %v, want a memory that is already on refused", err)
		}
	})

	t.Run("source erased", func(t *testing.T) {
		run, gone := disabledMemory(t, ctx, store, now)
		// The run is taken, which is what the retention sweep does. The first
		// reactivation records the state the row was already in and refuses;
		// the second meets that state and refuses again.
		run.erase()
		for _, attempt := range []string{"first", "second"} {
			_, err := store.Reactivate(ctx, run.resolver(), memory.ReactivateInput{
				ID: gone.ID, Scope: run.scope, By: "usr_bruno", Reason: "again", Now: now,
			})
			if !errors.Is(err, memory.ErrMemoryTerminal) {
				t.Fatalf("Reactivate (%s) = %v, want a memory with no source refused", attempt, err)
			}
		}
		if status := statusOf(t, ctx, store, run.scope, gone.ID); status != domain.MemorySourceErased {
			t.Errorf("status = %s, want the row to say its source is gone", status)
		}
	})
}

/*
A citation the ledger will not vouch for refuses the reactivation.

The run is still there; what it holds is not what the memory says it holds. That
is not a source erased and it is not a state a person can decide their way out
of — it is a memory the platform can no longer show the proof of, and switching
it back on would put it in front of a run as if it could.
*/
func TestReactivate_citationsThatNoLongerProveIt_isRefused(t *testing.T) {
	t.Parallel()
	expectUnprovableReactivationIsRefused(t, context.Background(), memory.NewMemory())
}

func TestPostgresReactivate_citationsThatNoLongerProveIt_isRefused(t *testing.T) {
	ctx, store := postgresStore(t)
	expectUnprovableReactivationIsRefused(t, ctx, store)
}

func expectUnprovableReactivationIsRefused(t *testing.T, ctx context.Context, store reactivateStore) {
	t.Helper()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	r, _ := finished(t, "run-mismatch")

	stored, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Scope, a.Subject = r.scope, "cited the wrong bytes"
		a.Evidence = []domain.MemoryEvidence{{
			RunID: r.id, Seq: 2, Artifact: domain.ArtifactFinalAnswer,
			Digest: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		}}
	}), "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}
	if err := store.Disable(ctx, stored.ID, r.scope, "usr_ana", "off", now); err != nil {
		t.Fatalf("Disable: %v", err)
	}

	if _, err := store.Reactivate(ctx, r.resolver(), memory.ReactivateInput{
		ID: stored.ID, Scope: r.scope, By: "usr_bruno", Reason: "again", Now: now,
	}); !errors.Is(err, memory.ErrEvidenceInvalid) {
		t.Fatalf("Reactivate = %v, want a memory the ledger will not vouch for refused", err)
	}
	if status := statusOf(t, ctx, store, r.scope, stored.ID); status != domain.MemoryDisabled {
		t.Errorf("status = %s, want the refused row left as it was", status)
	}
}

/*
A reactivation nobody explained is not a governance act.

The event carries the reason, and recordEvent trims and accepts an empty one —
so without this the trail would hold a row that came back to life beside a blank
where the justification should be.
*/
func TestReactivate_withoutAReason_isRefused(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := memory.NewMemory()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	run, disabled := disabledMemory(t, ctx, store, now)

	if _, err := store.Reactivate(ctx, run.resolver(), memory.ReactivateInput{
		ID: disabled.ID, Scope: run.scope, By: "usr_bruno", Reason: "   ", Now: now,
	}); !errors.Is(err, memory.ErrInvalid) {
		t.Fatalf("Reactivate = %v, want a reactivation without a reason refused", err)
	}
}

/*
Shared memory that answers the identity refuses the reactivation, and nothing
moves.

Bringing the agent's copy back would put two memories of one fact in front of
the same run — the shared one every agent reads, and a private one somebody
switched off months ago. The person is told which memory already answers this,
and goes to that one.
*/
func TestReactivate_sharedMemoryAlreadyCoversIt_isRefusedWithoutMutation(t *testing.T) {
	t.Parallel()
	expectCoveredReactivationIsRefused(t, context.Background(), memory.NewMemory())
}

func TestPostgresReactivate_sharedMemoryAlreadyCoversIt_isRefusedWithoutMutation(t *testing.T) {
	ctx, store := postgresStore(t)
	expectCoveredReactivationIsRefused(t, ctx, store)
}

func expectCoveredReactivationIsRefused(t *testing.T, ctx context.Context, store reactivateStore) {
	t.Helper()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	run, disabled := disabledMemory(t, ctx, store, now)

	shared, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Scope, a.AgentID = run.scope, ""
		a.Subject, a.Signature = disabled.Subject, disabled.Signature
		a.Evidence = slices.Clone(disabled.Evidence)
	}), "usr_ana", "the same fact, for everybody", now)
	if err != nil {
		t.Fatalf("Assert shared: %v", err)
	}

	_, err = store.Reactivate(ctx, run.resolver(), memory.ReactivateInput{
		ID: disabled.ID, Scope: run.scope, By: "usr_bruno", Reason: "again", Now: now,
	})
	if !errors.Is(err, memory.ErrCovered) {
		t.Fatalf("Reactivate = %v, want the shared memory to answer it", err)
	}
	if !strings.Contains(err.Error(), shared.ID) {
		t.Errorf("error = %v, want it to name the memory that already answers this", err)
	}
	if status := statusOf(t, ctx, store, run.scope, disabled.ID); status != domain.MemoryDisabled {
		t.Errorf("status = %s, want the refused row left as it was", status)
	}
}

/*
A repair that lands mid-decision makes the reactivation look again.

The ledger is read before the identity is held, so a hydration can fill the
citations between the snapshot and the write. Its version is newer than what
this decision was made about — the proof was of the old citations — and
reactivating anyway would vouch for evidence nobody in this transaction looked
at. Refusing is cheap: the person tries again and the second attempt proves what
is actually there.
*/
func TestPostgresReactivate_overtakenByARepair_refusesTheStaleSnapshot(t *testing.T) {
	ctx, pool := postgresPool(t)
	store := memory.NewPostgres(pool)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	r, cited := finished(t, "run-overtaken")
	resolved, err := r.resolver().Resolve(ctx, r.scope, []domain.MemoryEvidence{cited})
	if err != nil {
		t.Fatalf("resolve for the fixture: %v", err)
	}
	stored, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Scope, a.Labels = r.scope, resolved[0].Labels
		// The older shape: run and artifact, the whole digest, no step. What a
		// hydration is going to complete while the reactivation is deciding.
		a.Evidence = []domain.MemoryEvidence{{
			RunID: r.id, Artifact: domain.ArtifactFinalAnswer, Digest: resolved[0].Digest,
		}}
	}), "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}
	if err := store.Disable(ctx, stored.ID, r.scope, "usr_ana", "off", now); err != nil {
		t.Fatalf("Disable: %v", err)
	}

	blocked := &blockingContent{
		inner: r.content, entered: make(chan struct{}), release: make(chan struct{}),
	}
	done := make(chan error, 1)
	go func() {
		_, err := store.Reactivate(ctx, memory.NewResolver(r.ledger, blocked),
			memory.ReactivateInput{
				ID: stored.ID, Scope: r.scope, By: "usr_bruno",
				Reason: "it is true again", Now: now,
			})
		done <- err
	}()

	<-blocked.entered
	if _, err := store.Hydrate(ctx, r.resolver(),
		memory.HydratePage{Limit: 10, Now: now}); err != nil {
		t.Fatalf("Hydrate while reactivating: %v", err)
	}
	close(blocked.release)

	if err := <-done; !errors.Is(err, memory.ErrMovedMeanwhile) {
		t.Fatalf("Reactivate = %v, want the stale snapshot refused", err)
	}
	if status := statusOf(t, ctx, store, r.scope, stored.ID); status != domain.MemoryDisabled {
		t.Errorf("status = %s, want the memory left off until somebody decides again", status)
	}
}

/*
Two people deciding at once bring the memory back exactly once.

The status is checked before the ledger is read, which is the cheap answer and
not the authoritative one: between that read and the lock, somebody else's
reactivation can land. The second decision then arrives at a memory that is
already active — a state it has no business turning back on, and one that would
otherwise be written a second time with a second event, so the trail would show
two people bringing back a memory only one of them found switched off.
*/
func TestPostgresReactivate_racingAnotherDecision_bringsItBackOnce(t *testing.T) {
	ctx, pool := postgresPool(t)
	store := memory.NewPostgres(pool)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	r, disabled := disabledMemory(t, ctx, store, now)

	blocked := &blockingContent{
		inner: r.content, entered: make(chan struct{}), release: make(chan struct{}),
	}
	done := make(chan error, 1)
	go func() {
		_, err := store.Reactivate(ctx, memory.NewResolver(r.ledger, blocked),
			memory.ReactivateInput{
				ID: disabled.ID, Scope: r.scope, By: "usr_bruno",
				Reason: "it is true again", Now: now,
			})
		done <- err
	}()

	<-blocked.entered
	if _, err := store.Reactivate(ctx, r.resolver(), memory.ReactivateInput{
		ID: disabled.ID, Scope: r.scope, By: "usr_carla",
		Reason: "I got here first", Now: now,
	}); err != nil {
		t.Fatalf("the unblocked reactivation failed: %v", err)
	}
	close(blocked.release)

	if err := <-done; !errors.Is(err, memory.ErrMemoryTerminal) {
		t.Fatalf("the second decision = %v, want a memory that is already on refused", err)
	}
	var events int
	if err := pool.QueryRow(ctx, `
		select count(*) from memory_assertion_events
		where assertion_id = $1 and action = 'reactivated'`, disabled.ID).Scan(&events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if events != 1 {
		t.Errorf("reactivated events = %d, want the one decision that found it off", events)
	}
}

/*
A reactivation that fails after projecting leaves nothing.

The proof has to come from inside the transaction: a failure before the first
write proves only that nothing started. So the event insert is refused by a
trigger, which fires after the row has already been flipped back to active — and
if any of it survived, the memory would be readable again with nothing in the
trail saying who decided that or why.
*/
func TestPostgresReactivate_failureAfterProjecting_leavesNothing(t *testing.T) {
	ctx, pool := postgresPool(t)
	store := memory.NewPostgres(pool)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	r, disabled := disabledMemory(t, ctx, store, now)

	if _, err := pool.Exec(ctx, `
		create or replace function refuse_memory_event() returns trigger as $$
		begin raise exception 'refused for the test'; end; $$ language plpgsql;
		create trigger refuse_memory_event before insert on memory_assertion_events
		for each row execute function refuse_memory_event();`); err != nil {
		t.Fatalf("install trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`drop trigger if exists refuse_memory_event on memory_assertion_events`)
	})

	if _, err := store.Reactivate(ctx, r.resolver(), memory.ReactivateInput{
		ID: disabled.ID, Scope: r.scope, By: "usr_bruno",
		Reason: "it is true again", Now: now,
	}); err == nil {
		t.Fatal("the reactivation succeeded while the event was being refused")
	}
	if status := statusOf(t, ctx, store, r.scope, disabled.ID); status != domain.MemoryDisabled {
		t.Errorf("status = %s, want the projection rolled back with the event", status)
	}
}

// disabledMemory is a memory somebody recorded from a run and then switched
// off, which is the only starting point a reactivation has.
func disabledMemory(
	t *testing.T, ctx context.Context, store reactivateStore, now time.Time,
) (*run, domain.MemoryAssertion) {
	t.Helper()
	r, cited := finished(t, domain.RunID("run-reactivate-"+t.Name()))
	resolved, err := r.resolver().Resolve(ctx, r.scope, []domain.MemoryEvidence{cited})
	if err != nil {
		t.Fatalf("resolve for the fixture: %v", err)
	}
	stored, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Scope, a.Evidence, a.Labels = r.scope, resolved, resolved[0].Labels
	}), "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}
	if err := store.Disable(ctx, stored.ID, r.scope, "usr_ana",
		"it stopped being true", now.Add(time.Hour)); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	return r, stored
}

func statusOf(
	t *testing.T, ctx context.Context, store reactivateStore,
	scope domain.Scope, id string,
) domain.MemoryStatus {
	t.Helper()
	rows, err := store.List(ctx, memory.Filter{Scopes: []domain.Scope{scope}, Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, row := range rows {
		if row.ID == id {
			return row.Status
		}
	}
	t.Fatalf("assertion %s is not in the store any more", id)
	return ""
}
