package ticket_test

import (
	"errors"
	"os"
	"strconv"
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

func TestStore_boundsOpenTicketsPerScope_andAClosedOneFreesItsPlace(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		var first ticket.Ticket
		for i := range ticket.MaxOpenPerScope {
			in := opening("event-cap-" + strconv.Itoa(i))
			in.Origin.Root = "root-cap-" + strconv.Itoa(i)
			in.Key, _ = ticket.Key(in.Origin.Connection, in.Origin.Conversation, in.Origin.Root)
			opened, _, err := store.Open(t.Context(), in)
			if err != nil {
				t.Fatalf("Open %d: %v", i, err)
			}
			if i == 0 {
				first = opened
			}
		}
		over := opening("event-cap-over")
		over.Origin.Root = "root-cap-over"
		over.Key, _ = ticket.Key(over.Origin.Connection, over.Origin.Conversation, over.Origin.Root)
		if _, _, err := store.Open(t.Context(), over); !errors.Is(err, ticket.ErrTooManyOpen) {
			t.Fatalf("Open above cap: %v, want ErrTooManyOpen", err)
		}
		if _, _, err := store.Close(t.Context(), ticket.CloseInput{
			Ref: first.Current.Ref, Phase: ticket.PhaseCancelled,
			Result: content("cancelled-cap"), At: now.Add(time.Hour),
		}); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if _, _, err := store.Open(t.Context(), over); err != nil {
			t.Fatalf("Open after close: %v", err)
		}
	})
}

func TestStore_aTerminalOutcomeIsLeasedUntilItsThreadIsTold(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		notices, ok := store.(ticket.OutcomeNotices)
		if !ok {
			t.Fatal("store does not expose terminal outcome notifications")
		}
		opened := mustOpen(t, store, "event-outcome")
		result := content("safe-outcome")
		if _, _, err := store.Close(t.Context(), ticket.CloseInput{
			Ref: opened.Current.Ref, Phase: ticket.PhaseRejected,
			Result: result, At: now.Add(time.Minute),
		}); err != nil {
			t.Fatalf("Close: %v", err)
		}

		claimed, err := notices.ClaimOutcomes(t.Context(), "worker-a", now.Add(2*time.Minute), time.Minute, 10)
		if err != nil || len(claimed) != 1 || claimed[0].Ref != opened.Current.Ref ||
			claimed[0].Origin != opened.Origin || claimed[0].Phase != ticket.PhaseRejected ||
			claimed[0].Result != result {
			t.Fatalf("first claim = (%+v, %v)", claimed, err)
		}
		if other, err := notices.ClaimOutcomes(t.Context(), "worker-b", now.Add(2*time.Minute), time.Minute, 10); err != nil || len(other) != 0 {
			t.Fatalf("claim inside lease = (%+v, %v), want none", other, err)
		}
		claimed, err = notices.ClaimOutcomes(t.Context(), "worker-b", now.Add(4*time.Minute), time.Minute, 10)
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim after lease = (%+v, %v)", claimed, err)
		}
		if err := notices.MarkOutcomeAnnounced(t.Context(), opened.Current.Ref, "worker-a", now.Add(4*time.Minute)); !errors.Is(err, ticket.ErrMoved) {
			t.Fatalf("superseded worker marked outcome = %v, want ErrMoved", err)
		}
		if err := notices.MarkOutcomeAnnounced(t.Context(), opened.Current.Ref, "worker-b", now.Add(4*time.Minute)); err != nil {
			t.Fatalf("MarkOutcomeAnnounced: %v", err)
		}
		if err := notices.MarkOutcomeAnnounced(t.Context(), opened.Current.Ref, "worker-b", now.Add(5*time.Minute)); err != nil {
			t.Fatalf("replayed MarkOutcomeAnnounced: %v", err)
		}
		if again, err := notices.ClaimOutcomes(t.Context(), "worker-c", now.Add(6*time.Minute), time.Minute, 10); err != nil || len(again) != 0 {
			t.Fatalf("claim after announcement = (%+v, %v), want none", again, err)
		}
	})
}

func TestStore_manualAttentionKeepsTheClaimAndAConfirmedResultIsAnnouncedAgain(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		notices := store.(ticket.OutcomeNotices)
		opened := mustOpen(t, store, "event-attention")
		mustInspect(t, store, opened.Current.Ref, content("snapshot-attention"), now.Add(time.Minute))
		if _, _, err := store.AwaitApproval(t.Context(), ticket.ApprovalInput{
			Ref: opened.Current.Ref, RunID: "run-attention", AtSeq: 7,
			Snapshot: content("snapshot-attention"), At: now.Add(2 * time.Minute),
		}); err != nil {
			t.Fatalf("AwaitApproval: %v", err)
		}
		claimed, _, err := store.ClaimExecution(t.Context(), ticket.ClaimInput{
			Ref: opened.Current.Ref, RunID: "run-attention", ApprovalAtSeq: 7,
			At: now.Add(3 * time.Minute),
		})
		if err != nil || claimed.Active == nil {
			t.Fatalf("ClaimExecution = (%+v, %v)", claimed, err)
		}
		execution := *claimed.Active
		attention := content("needs-attention")
		current, err := store.MarkExecutionNeedsAttention(t.Context(), ticket.AttentionInput{
			Execution: execution, Result: attention, At: now.Add(4 * time.Minute),
		})
		if err != nil || current.Active == nil || current.Current.Phase != ticket.PhaseNeedsAttention {
			t.Fatalf("MarkExecutionNeedsAttention = (%+v, %v)", current, err)
		}
		first, err := notices.ClaimOutcomes(t.Context(), "worker-a", now.Add(5*time.Minute), time.Minute, 10)
		if err != nil || len(first) != 1 || first[0].Phase != ticket.PhaseNeedsAttention {
			t.Fatalf("attention notice = (%+v, %v)", first, err)
		}
		if err := notices.MarkOutcomeAnnounced(t.Context(), opened.Current.Ref,
			"worker-a", now.Add(5*time.Minute)); err != nil {
			t.Fatalf("mark attention: %v", err)
		}

		final := content("accepted-after-attention")
		current, err = store.FinishExecution(t.Context(), ticket.FinishInput{
			Execution: execution, Phase: ticket.PhaseCompleted,
			Result: final, At: now.Add(6 * time.Minute),
		})
		if err != nil || current.Active != nil || current.Current.Phase != ticket.PhaseCompleted {
			t.Fatalf("FinishExecution = (%+v, %v)", current, err)
		}
		second, err := notices.ClaimOutcomes(t.Context(), "worker-b", now.Add(7*time.Minute), time.Minute, 10)
		if err != nil || len(second) != 1 || second[0].Phase != ticket.PhaseCompleted ||
			second[0].Result != final {
			t.Fatalf("final notice = (%+v, %v)", second, err)
		}
	})
}

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

func TestStore_aTicketIsFoundOnlyAtItsExactSlackOrigin(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		opened := mustOpen(t, store, "event-root")
		got, err := store.AtOrigin(t.Context(), ticket.Origin{
			Connection: "slack-main", Conversation: "C-support", Root: "171.22",
		})
		if err != nil || got.Key != opened.Key {
			t.Fatalf("AtOrigin = (%+v, %v), want %s", got, err, opened.Key)
		}

		for _, wrong := range []ticket.Origin{
			{Connection: "slack-other", Conversation: "C-support", Root: "171.22"},
			{Connection: "slack-main", Conversation: "C-other", Root: "171.22"},
			{Connection: "slack-main", Conversation: "C-support", Root: "172.33"},
		} {
			if _, err := store.AtOrigin(t.Context(), wrong); !errors.Is(err, ticket.ErrNotFound) {
				t.Fatalf("AtOrigin(%+v) = %v, want ErrNotFound", wrong, err)
			}
		}
	})
}

func TestStore_approvalRouteReadsTheExactRevisionWithoutSharingItsRecipients(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		routes, ok := store.(ticket.ApprovalRoutes)
		if !ok {
			t.Fatal("store does not expose ticket approval routes")
		}
		opened := mustOpen(t, store, "event-route")
		addressed, changed, err := store.Address(t.Context(), ticket.AddressInput{
			Ref: opened.Current.Ref, EventID: "event-route-address",
			By: "app:ticket-bot", Recipients: []domain.UserID{"usr_manager"},
			At: now.Add(time.Minute),
		})
		if err != nil || !changed {
			t.Fatalf("Address = (%+v, %v, %v)", addressed, changed, err)
		}

		route, err := routes.ApprovalRoute(t.Context(), addressed.Current.Ref)
		if err != nil || route.Origin != opened.Origin ||
			len(route.Recipients) != 1 || route.Recipients[0] != "usr_manager" {
			t.Fatalf("ApprovalRoute = (%+v, %v)", route, err)
		}
		route.Recipients[0] = "usr_intruder"
		again, err := routes.ApprovalRoute(t.Context(), addressed.Current.Ref)
		if err != nil || len(again.Recipients) != 1 || again.Recipients[0] != "usr_manager" {
			t.Fatalf("caller changed stored route: (%+v, %v)", again, err)
		}
		before, err := routes.ApprovalRoute(t.Context(), opened.Current.Ref)
		if err != nil || len(before.Recipients) != 0 {
			t.Fatalf("older revision inherited recipients: (%+v, %v)", before, err)
		}
	})
}

func TestStore_anEventNamesTheRevisionItActuallyCreated(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		opened := mustOpen(t, store, "event-root")
		revised, _, err := store.Revise(t.Context(), ticket.ReviseInput{
			Ref: opened.Current.Ref, EventID: "event-reply", By: "usr_requester",
			Draft: content("draft-2"), At: now.Add(time.Minute),
		})
		if err != nil {
			t.Fatalf("Revise: %v", err)
		}
		_, _, err = store.Address(t.Context(), ticket.AddressInput{
			Ref: revised.Current.Ref, EventID: "event-address", By: "app:ticket-bot",
			Recipients: []domain.UserID{"usr_ana"}, At: now.Add(2 * time.Minute),
		})
		if err != nil {
			t.Fatalf("Address: %v", err)
		}

		got, revision, err := store.EventRevision(t.Context(), "event-reply")
		if err != nil || got.Key != opened.Key || revision.Ref.Revision != 2 ||
			revision.Draft != content("draft-2") {
			t.Fatalf("EventRevision = (%+v, %+v, %v)", got, revision, err)
		}
	})
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

func TestStore_anExecutionAndItsExternalAttemptAreOneClaim(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		first := mustAwaiting(t, store, "atomic-first", "run-atomic-first", 7)
		input := claimWithAttempt(first.Current.Ref, "run-atomic-first", 7, "effect-once")
		claimed, attempt, won, err := store.ClaimExecutionWithAttempt(t.Context(), input)
		if err != nil || !won || claimed.Active == nil || attempt.Execution != *claimed.Active {
			t.Fatalf("ClaimExecutionWithAttempt = (%+v, %+v, %v, %v)", claimed, attempt, won, err)
		}
		if replay, same, won, err := store.ClaimExecutionWithAttempt(t.Context(), input); err != nil || won || replay.Active == nil || same != attempt {
			t.Fatalf("replayed claim = (%+v, %+v, %v, %v)", replay, same, won, err)
		}
		changed := input
		changed.Attempt.TargetID = "another-target"
		if _, _, _, err := store.ClaimExecutionWithAttempt(t.Context(), changed); !errors.Is(err, ticket.ErrAttemptConflict) {
			t.Fatalf("changed attempt = %v, want ErrAttemptConflict", err)
		}

		second := mustAwaiting(t, store, "atomic-second", "run-atomic-second", 11)
		collision := claimWithAttempt(second.Current.Ref, "run-atomic-second", 11, "effect-once")
		if _, _, _, err := store.ClaimExecutionWithAttempt(t.Context(), collision); !errors.Is(err, ticket.ErrAttemptConflict) {
			t.Fatalf("colliding attempt = %v, want ErrAttemptConflict", err)
		}
		current, err := store.Current(t.Context(), second.Key)
		if err != nil || current.Active != nil || current.Current.Phase != ticket.PhaseAwaitingApproval {
			t.Fatalf("attempt collision left a partial claim: (%+v, %v)", current, err)
		}
	})
}

func mustAwaiting(
	t *testing.T, store ticket.Store, suffix string, run domain.RunID, atSeq int64,
) ticket.Ticket {
	t.Helper()
	key, err := ticket.Key("slack-main", "C-support", suffix)
	if err != nil {
		t.Fatalf("ticket key: %v", err)
	}
	opened, _, err := store.Open(t.Context(), ticket.OpenInput{
		Key: key, Origin: ticket.Origin{
			Connection: "slack-main", Conversation: "C-support", Root: suffix,
		}, Scope: scope, Agent: "gateway-support", RunAs: "usr_gateway",
		RequestedBy: "usr_requester", AddressedBy: "app:ticket-bot",
		EventID: "event-" + suffix, Draft: content("draft-" + suffix), At: now,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	snapshot := content("snapshot-" + suffix)
	mustInspect(t, store, opened.Current.Ref, snapshot, now.Add(30*time.Second))
	awaiting, _, err := store.AwaitApproval(t.Context(), ticket.ApprovalInput{
		Ref: opened.Current.Ref, RunID: run, AtSeq: atSeq,
		Snapshot: snapshot, At: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("AwaitApproval: %v", err)
	}
	return awaiting
}

func claimWithAttempt(
	ref domain.TicketRef, run domain.RunID, atSeq int64, idemKey string,
) ticket.ClaimAttemptInput {
	started := now.Add(2 * time.Minute)
	return ticket.ClaimAttemptInput{
		Claim: ticket.ClaimInput{Ref: ref, RunID: run, ApprovalAtSeq: atSeq, At: started},
		Attempt: ticket.ExternalAttemptInput{
			IdemKey: idemKey, Kind: "test.effect", CallSeq: 13, Instance: "fixture",
			Scope: scope, ContractDigest: "contract:v1", TargetID: "target-1",
			DecidedBy: "usr_approver", NextCheckAt: started,
			DeadlineAt: started.Add(10 * time.Minute), ClaimedBy: "worker-1",
			ClaimedUntil: started.Add(time.Minute),
		},
	}
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
	origin := ticket.Origin{
		Connection: "slack-main", Conversation: "C-support", Root: "171.22",
	}
	key, err := ticket.Key(origin.Connection, origin.Conversation, origin.Root)
	if err != nil {
		panic(err)
	}
	return ticket.OpenInput{
		Key: key, Origin: origin, Scope: scope,
		Agent: "gateway-support", RunAs: "usr_gateway", RequestedBy: "usr_requester",
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
				`truncate governed_external_attempts, governed_ticket_events,
				 governed_ticket_revisions, governed_tickets`); err != nil {
				t.Fatalf("clean tickets: %v", err)
			}
			test(t, ticket.NewPostgres(pool))
		})
	} else if os.Getenv("REQUIRE_DATABASE") != "" {
		t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
	}
}
