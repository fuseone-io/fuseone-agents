package ticket_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ticket"
)

func TestStore_anEventCannotAdvanceTwoTickets(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		first, _, err := store.Open(t.Context(), opening("event-shared"))
		if err != nil {
			t.Fatalf("open first: %v", err)
		}
		second := opening("event-shared")
		second.Origin.Root = "172.33"
		second.Key, err = ticket.Key("slack-main", "C-support", "172.33")
		if err != nil {
			t.Fatalf("second key: %v", err)
		}
		if _, _, err := store.Open(t.Context(), second); !errors.Is(err, ticket.ErrEventTaken) {
			t.Fatalf("reuse event = %v, want ErrEventTaken", err)
		}
		current, err := store.Current(t.Context(), first.Key)
		if err != nil || current.Current.Ref.Revision != 1 {
			t.Fatalf("first ticket changed = (%+v, %v)", current, err)
		}
	})
}

func TestStore_anEventRacingAcrossTicketsBelongsToExactlyOne(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		inputs := []ticket.OpenInput{opening("event-racing"), opening("event-racing")}
		var err error
		inputs[1].Origin.Root = "172.33"
		inputs[1].Key, err = ticket.Key("slack-main", "C-support", "172.33")
		if err != nil {
			t.Fatalf("second key: %v", err)
		}
		var wg sync.WaitGroup
		results := make(chan bool, 2)
		errs := make(chan error, 2)
		for _, input := range inputs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, created, openErr := store.Open(t.Context(), input)
				results <- created
				errs <- openErr
			}()
		}
		wg.Wait()
		close(results)
		close(errs)
		created, refused := 0, 0
		for result := range results {
			if result {
				created++
			}
		}
		for err := range errs {
			switch {
			case err == nil:
			case errors.Is(err, ticket.ErrEventTaken):
				refused++
			default:
				t.Errorf("Open error = %v, want nil or ErrEventTaken", err)
			}
		}
		if created != 1 || refused != 1 {
			t.Fatalf("created %d and refused %d, want one each", created, refused)
		}
	})
}

func TestStore_onlyTheAddressingSourceMayReplaceRecipients(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		opened, _, err := store.Open(t.Context(), opening("event-root"))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		_, _, err = store.Address(t.Context(), ticket.AddressInput{
			Ref: opened.Current.Ref, EventID: "event-address-wrong",
			By: "app:another", Recipients: []domain.UserID{"usr_ana"},
			At: now.Add(time.Minute),
		})
		if !errors.Is(err, ticket.ErrNotAddressSource) {
			t.Fatalf("wrong source = %v, want ErrNotAddressSource", err)
		}

		addressed, changed, err := store.Address(t.Context(), ticket.AddressInput{
			Ref: opened.Current.Ref, EventID: "event-address",
			By:         "app:ticket-bot",
			Recipients: []domain.UserID{"usr_bia", "usr_ana", "usr_bia"},
			At:         now.Add(2 * time.Minute),
		})
		if err != nil || !changed || addressed.Current.Ref.Revision != 2 {
			t.Fatalf("Address = (%+v, %v, %v)", addressed, changed, err)
		}
		want := []domain.UserID{"usr_ana", "usr_bia"}
		if !equalUsers(addressed.Current.Recipients, want) ||
			addressed.Current.Draft != content("draft-1") {
			t.Fatalf("addressed revision = %+v, want recipients %v and original draft",
				addressed.Current, want)
		}

		same, changed, err := store.Address(t.Context(), ticket.AddressInput{
			Ref: addressed.Current.Ref, EventID: "event-same-address",
			By:         "app:ticket-bot",
			Recipients: []domain.UserID{"usr_bia", "usr_ana"},
			At:         now.Add(3 * time.Minute),
		})
		if err != nil || changed || same.Current.Ref.Revision != 2 {
			t.Fatalf("same Address = (%+v, %v, %v)", same, changed, err)
		}
	})
}

func TestStore_aReturnedRecipientListCannotMutateTheStore(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		opened := mustOpen(t, store, "event-root")
		addressed, changed, err := store.Address(t.Context(), ticket.AddressInput{
			Ref: opened.Current.Ref, EventID: "event-address",
			By: "app:ticket-bot", Recipients: []domain.UserID{"usr_ana"},
			At: now.Add(time.Minute),
		})
		if err != nil || !changed {
			t.Fatalf("Address = (%+v, %v, %v)", addressed, changed, err)
		}
		addressed.Current.Recipients[0] = "usr_intruder"
		current, err := store.Current(t.Context(), opened.Key)
		if err != nil {
			t.Fatalf("Current: %v", err)
		}
		if !equalUsers(current.Current.Recipients, []domain.UserID{"usr_ana"}) {
			t.Fatalf("caller changed stored recipients: %v", current.Current.Recipients)
		}
	})
}

func TestStore_supersededApprovalsNamesEveryCancelledQuestionInOrder(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		current := mustOpen(t, store, "event-root")
		for i, runID := range []domain.RunID{"run-1", "run-2"} {
			revision := int64(i + 1)
			snapshot := []ticket.ContentRef{content("snapshot-1"), content("snapshot-2")}[i]
			mustInspect(t, store, current.Current.Ref, snapshot,
				now.Add(time.Duration(revision)*time.Minute))
			if _, _, err := store.AwaitApproval(t.Context(), ticket.ApprovalInput{
				Ref:   current.Current.Ref,
				RunID: runID,
				AtSeq: revision + 6, Snapshot: snapshot,
				At: now.Add(time.Duration(revision) * time.Minute),
			}); err != nil {
				t.Fatalf("AwaitApproval revision %d: %v", revision, err)
			}
			var err error
			current, _, err = store.Revise(t.Context(), ticket.ReviseInput{
				Ref:     current.Current.Ref,
				EventID: []string{"event-revision-1", "event-revision-2"}[i],
				By:      current.RequestedBy, Draft: current.Current.Draft,
				At: now.Add(time.Duration(revision+2) * time.Minute),
			})
			if err != nil {
				t.Fatalf("Revise %d: %v", revision, err)
			}
		}

		approvals, err := store.SupersededApprovals(t.Context(), current.Current.Ref)
		if err != nil {
			t.Fatalf("SupersededApprovals: %v", err)
		}
		if len(approvals) != 2 ||
			approvals[0].RunID != "run-1" || approvals[0].AtSeq != 7 ||
			approvals[1].RunID != "run-2" || approvals[1].AtSeq != 8 {
			t.Fatalf("approvals = %+v, want both cancelled questions in revision order", approvals)
		}
		if err := store.MarkApprovalSuperseded(
			t.Context(), approvals[0].Ref, now.Add(5*time.Minute),
		); err != nil {
			t.Fatalf("MarkApprovalSuperseded: %v", err)
		}
		remaining, err := store.SupersededApprovals(t.Context(), current.Current.Ref)
		if err != nil || len(remaining) != 1 || remaining[0].RunID != "run-2" {
			t.Fatalf("remaining = (%+v, %v), want only the unsettled obligation", remaining, err)
		}
	})
}

func TestStore_aRefusedApprovalNeverBecomesExecutable(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		opened := mustOpen(t, store, "event-root")
		mustInspect(t, store, opened.Current.Ref, content("snapshot-1"), now.Add(30*time.Second))
		awaiting, changed, err := store.AwaitApproval(t.Context(), ticket.ApprovalInput{
			Ref: opened.Current.Ref, RunID: "run-1", AtSeq: 7,
			Snapshot: content("snapshot-1"), At: now.Add(time.Minute),
		})
		if err != nil || !changed {
			t.Fatalf("AwaitApproval = (%+v, %v, %v)", awaiting, changed, err)
		}
		closed, changed, err := store.Close(t.Context(), ticket.CloseInput{
			Ref: opened.Current.Ref, Phase: ticket.PhaseRejected,
			Result: content("decision-1"), At: now.Add(2 * time.Minute),
		})
		if err != nil || !changed || closed.Current.Phase != ticket.PhaseRejected {
			t.Fatalf("Close = (%+v, %v, %v)", closed, changed, err)
		}
		_, _, err = store.ClaimExecution(t.Context(), ticket.ClaimInput{
			Ref: opened.Current.Ref, RunID: "run-1", ApprovalAtSeq: 7,
			At: now.Add(3 * time.Minute),
		})
		if !errors.Is(err, ticket.ErrTerminal) {
			t.Fatalf("claim rejected revision = %v, want ErrTerminal", err)
		}
	})
}

func TestStore_anApprovalCanOnlyNameTheSnapshotRecordedForThatRevision(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		opened := mustOpen(t, store, "event-root")
		mustInspect(t, store, opened.Current.Ref, content("snapshot-seen"), now.Add(time.Minute))

		_, changed, err := store.AwaitApproval(t.Context(), ticket.ApprovalInput{
			Ref: opened.Current.Ref, RunID: "run-1", AtSeq: 7,
			Snapshot: content("snapshot-invented"), At: now.Add(2 * time.Minute),
		})
		if changed || !errors.Is(err, ticket.ErrSnapshotMoved) {
			t.Fatalf("AwaitApproval = (%v, %v), want ErrSnapshotMoved", changed, err)
		}
		current, err := store.Current(t.Context(), opened.Key)
		if err != nil {
			t.Fatalf("Current: %v", err)
		}
		if current.Current.Phase != ticket.PhaseCollecting ||
			current.Current.Snapshot != content("snapshot-seen") {
			t.Fatalf("arbitrary snapshot changed the ticket: %+v", current.Current)
		}
	})
}

func mustOpen(t *testing.T, store ticket.Store, event string) ticket.Ticket {
	t.Helper()
	opened, created, err := store.Open(t.Context(), opening(event))
	if err != nil || !created {
		t.Fatalf("Open = (%+v, %v, %v)", opened, created, err)
	}
	return opened
}

func equalUsers(a, b []domain.UserID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
