package ticket_test

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/ticket"
)

var (
	now   = time.Date(2026, 9, 11, 18, 0, 0, 0, time.UTC)
	scope = domain.Scope{Company: "acme", Area: "platform"}
)

func mustInspect(
	t *testing.T, store ticket.Store, ref domain.TicketRef, snapshot ticket.ContentRef, at time.Time,
) {
	t.Helper()
	got, changed, err := store.RecordInspection(t.Context(), ticket.InspectionInput{
		Ref: ref, Snapshot: snapshot, At: at,
	})
	if err != nil || !changed || got.Current.Snapshot != snapshot {
		t.Fatalf("RecordInspection = (%+v, %v, %v)", got, changed, err)
	}
}

func TestKey_isInjectiveAcrossThreeVendorNamespaces(t *testing.T) {
	t.Parallel()
	a, err := ticket.Key("workspace", "support/team", "171.22")
	if err != nil {
		t.Fatalf("first key: %v", err)
	}
	b, err := ticket.Key("workspace/support", "team", "171.22")
	if err != nil {
		t.Fatalf("second key: %v", err)
	}
	if a == b {
		t.Fatalf("different Slack threads share key %q", a)
	}
	if _, err := ticket.Key("workspace", "", "171.22"); err == nil {
		t.Fatal("a ticket key accepted an empty conversation")
	}
	if _, err := ticket.Key(strings.Repeat("w", 513), "support", "171.22"); err == nil {
		t.Fatal("a ticket key accepted an unbounded vendor namespace")
	}
}

func TestStore_anEventReplayNeverAdvancesTheRevisionTwice(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		opened, created, err := store.Open(t.Context(), opening("event-root"))
		if err != nil || !created {
			t.Fatalf("Open = (%+v, %v, %v)", opened, created, err)
		}
		if opened.Current.Ref.Revision != 1 {
			t.Fatalf("initial revision = %d, want 1", opened.Current.Ref.Revision)
		}

		again, created, err := store.Open(t.Context(), opening("event-root"))
		if err != nil || created || again.Current.Ref != opened.Current.Ref {
			t.Fatalf("replayed Open = (%+v, %v, %v)", again, created, err)
		}

		revised, changed, err := store.Revise(t.Context(), ticket.ReviseInput{
			Ref: opened.Current.Ref, EventID: "event-reply", By: "usr_requester",
			Draft: content("draft-2"), At: now.Add(time.Minute),
		})
		if err != nil || !changed || revised.Current.Ref.Revision != 2 {
			t.Fatalf("Revise = (%+v, %v, %v)", revised, changed, err)
		}

		replay, changed, err := store.Revise(t.Context(), ticket.ReviseInput{
			Ref: opened.Current.Ref, EventID: "event-reply", By: "usr_requester",
			Draft: content("different-body"), At: now.Add(2 * time.Minute),
		})
		if err != nil || changed || replay.Current.Ref.Revision != 2 ||
			replay.Current.Draft != content("draft-2") {
			t.Fatalf("replayed Revise = (%+v, %v, %v)", replay, changed, err)
		}

		_, _, err = store.Revise(t.Context(), ticket.ReviseInput{
			Ref: opened.Current.Ref, EventID: "event-new", By: "usr_requester",
			Draft: content("draft-3"), At: now.Add(3 * time.Minute),
		})
		if !errors.Is(err, ticket.ErrMoved) {
			t.Fatalf("stale revision = %v, want ErrMoved", err)
		}
	})
}

func TestStore_onlyTheRequesterMayReviseTheTicket(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		opened, _, err := store.Open(t.Context(), opening("event-root"))
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		_, _, err = store.Revise(t.Context(), ticket.ReviseInput{
			Ref: opened.Current.Ref, EventID: "event-stranger", By: "usr_stranger",
			Draft: content("stolen"), At: now.Add(time.Minute),
		})
		if !errors.Is(err, ticket.ErrNotRequester) {
			t.Fatalf("stranger revise = %v, want ErrNotRequester", err)
		}
		got, err := store.Current(t.Context(), opened.Key)
		if err != nil {
			t.Fatalf("Current: %v", err)
		}
		if got.Current.Ref.Revision != 1 || got.Current.Draft != content("draft-1") {
			t.Fatalf("stranger changed ticket: %+v", got)
		}
	})
}

func TestStore_oneApprovalBecomesOneExecutionClaim(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		opened := mustOpen(t, store, "event-root")
		mustInspect(t, store, opened.Current.Ref, content("snapshot-1"), now.Add(30*time.Second))
		awaiting, changed, err := store.AwaitApproval(t.Context(), ticket.ApprovalInput{
			Ref: opened.Current.Ref, RunID: "run-1", AtSeq: 7,
			Snapshot: content("snapshot-1"), At: now.Add(time.Minute),
		})
		if err != nil || !changed || awaiting.Current.Phase != ticket.PhaseAwaitingApproval {
			t.Fatalf("AwaitApproval = (%+v, %v, %v)", awaiting, changed, err)
		}

		input := ticket.ClaimInput{
			Ref: opened.Current.Ref, RunID: "run-1", ApprovalAtSeq: 7,
			At: now.Add(2 * time.Minute),
		}
		var wg sync.WaitGroup
		results := make(chan bool, 8)
		errs := make(chan error, 8)
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, claimed, claimErr := store.ClaimExecution(t.Context(), input)
				results <- claimed
				errs <- claimErr
			}()
		}
		wg.Wait()
		close(results)
		close(errs)
		claimed := 0
		for result := range results {
			if result {
				claimed++
			}
		}
		for claimErr := range errs {
			if claimErr != nil {
				t.Fatalf("ClaimExecution: %v", claimErr)
			}
		}
		if claimed != 1 {
			t.Fatalf("claimed = %d, want exactly one", claimed)
		}
	})
}

func TestStore_anOlderExecutionCannotCompleteANewerRevision(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		opened := mustOpen(t, store, "event-root")
		mustInspect(t, store, opened.Current.Ref, content("snapshot-1"), now.Add(30*time.Second))
		_, changed, err := store.AwaitApproval(t.Context(), ticket.ApprovalInput{
			Ref: opened.Current.Ref, RunID: "run-1", AtSeq: 7,
			Snapshot: content("snapshot-1"), At: now.Add(time.Minute),
		})
		if err != nil || !changed {
			t.Fatalf("AwaitApproval = (%v, %v)", changed, err)
		}
		claimed, won, err := store.ClaimExecution(t.Context(), ticket.ClaimInput{
			Ref: opened.Current.Ref, RunID: "run-1", ApprovalAtSeq: 7,
			At: now.Add(2 * time.Minute),
		})
		if err != nil || !won || claimed.Active == nil {
			t.Fatalf("ClaimExecution = (%+v, %v, %v)", claimed, won, err)
		}
		execution := *claimed.Active

		newer, changed, err := store.Revise(t.Context(), ticket.ReviseInput{
			Ref: opened.Current.Ref, EventID: "event-reply", By: "usr_requester",
			Draft: content("draft-2"), At: now.Add(3 * time.Minute),
		})
		if err != nil || !changed || newer.Active == nil || newer.Current.Ref.Revision != 2 {
			t.Fatalf("Revise during execution = (%+v, %v, %v)", newer, changed, err)
		}
		mustInspect(t, store, newer.Current.Ref, content("snapshot-2"), now.Add(3*time.Minute+30*time.Second))
		newer, _, err = store.AwaitApproval(t.Context(), ticket.ApprovalInput{
			Ref: newer.Current.Ref, RunID: "run-2", AtSeq: 11,
			Snapshot: content("snapshot-2"), At: now.Add(4 * time.Minute),
		})
		if err != nil {
			t.Fatalf("AwaitApproval newer: %v", err)
		}
		_, _, err = store.ClaimExecution(t.Context(), ticket.ClaimInput{
			Ref: newer.Current.Ref, RunID: "run-2", ApprovalAtSeq: 11,
			At: now.Add(5 * time.Minute),
		})
		if !errors.Is(err, ticket.ErrExecutionActive) {
			t.Fatalf("second active execution = %v, want ErrExecutionActive", err)
		}

		current, err := store.FinishExecution(t.Context(), ticket.FinishInput{
			Execution: execution, Phase: ticket.PhaseCompleted,
			Result: content("result-1"), At: now.Add(6 * time.Minute),
		})
		if err != nil {
			t.Fatalf("FinishExecution: %v", err)
		}
		if current.Current.Ref.Revision != 2 ||
			current.Current.Phase != ticket.PhaseAwaitingApproval || current.Active != nil {
			t.Fatalf("old completion overwrote current ticket: %+v", current)
		}
		old, err := store.Revision(t.Context(), execution.Ref)
		if err != nil {
			t.Fatalf("old Revision: %v", err)
		}
		if old.Phase != ticket.PhaseCompleted || old.Outcome == nil ||
			old.Outcome.Result != content("result-1") {
			t.Fatalf("old outcome was not recorded on its revision: %+v", old)
		}
	})
}

func TestStore_finishingTheSameExecutionIsIdempotent(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		opened := mustOpen(t, store, "event-root")
		mustInspect(t, store, opened.Current.Ref, content("snapshot-1"), now.Add(30*time.Second))
		_, changed, err := store.AwaitApproval(t.Context(), ticket.ApprovalInput{
			Ref: opened.Current.Ref, RunID: "run-1", AtSeq: 7,
			Snapshot: content("snapshot-1"), At: now.Add(time.Minute),
		})
		if err != nil || !changed {
			t.Fatalf("AwaitApproval = (%v, %v)", changed, err)
		}
		claimed, won, err := store.ClaimExecution(t.Context(), ticket.ClaimInput{
			Ref: opened.Current.Ref, RunID: "run-1", ApprovalAtSeq: 7,
			At: now.Add(2 * time.Minute),
		})
		if err != nil || !won || claimed.Active == nil {
			t.Fatalf("ClaimExecution = (%+v, %v, %v)", claimed, won, err)
		}
		finish := ticket.FinishInput{
			Execution: *claimed.Active, Phase: ticket.PhaseCompleted,
			Result: content("result-1"), At: now.Add(3 * time.Minute),
		}
		if _, err := store.FinishExecution(t.Context(), finish); err != nil {
			t.Fatalf("first FinishExecution: %v", err)
		}
		if _, err := store.FinishExecution(t.Context(), finish); err != nil {
			t.Fatalf("replayed FinishExecution: %v", err)
		}
		finish.Result = content("different-result")
		if _, err := store.FinishExecution(t.Context(), finish); !errors.Is(err, ticket.ErrMoved) {
			t.Fatalf("different FinishExecution = %v, want ErrMoved", err)
		}
	})
}

func opening(event string) ticket.OpenInput {
	key, err := ticket.Key("slack-main", "C-support", "171.22")
	if err != nil {
		panic(err)
	}
	return ticket.OpenInput{
		Key: key, Scope: scope, RequestedBy: "usr_requester",
		AddressedBy: "app:ticket-bot", EventID: event,
		Draft: content("draft-1"), At: now,
	}
}

func content(name string) ticket.ContentRef {
	return ticket.ContentRef{Ref: "ticket://" + name, Digest: "sha256:" + name}
}

func forEachStore(t *testing.T, test func(*testing.T, ticket.Store)) {
	t.Helper()
	t.Run("memory", func(t *testing.T) { test(t, ticket.NewMemory()) })
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		t.Run("postgres", func(t *testing.T) {
			pool, err := pgxpool.New(t.Context(), dsn)
			if err != nil {
				t.Fatalf("connect: %v", err)
			}
			t.Cleanup(pool.Close)
			if err := ledger.Migrate(t.Context(), pool); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			if _, err := pool.Exec(t.Context(),
				`truncate governed_ticket_events, governed_ticket_revisions, governed_tickets`); err != nil {
				t.Fatalf("clean tickets: %v", err)
			}
			test(t, ticket.NewPostgres(pool))
		})
	} else if os.Getenv("REQUIRE_DATABASE") != "" {
		t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
	}
}
