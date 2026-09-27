package ticket_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ticket"
)

/*
Publishing an answer settles that revision, not the ticket.

A support request is not over because somebody answered it once: the next thing
the person says is about the same request, and the thread they say it in is the
same thread. So the revision that was approved stays completed — it is the
record of what was published — and the ticket goes on accepting corrections
until somebody says it is done.
*/
func TestStore_aPublishedRevision_leavesTheTicketOpen(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		in := opening("event-answered")
		held, _, err := store.Open(t.Context(), in)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		published := publishOneAnswer(t, store, held)

		// The answer's own revision keeps its outcome: it is what was said.
		settled, err := store.Revision(t.Context(), published)
		if err != nil || settled.Phase != ticket.PhaseCompleted || settled.Outcome == nil {
			t.Fatalf("revision %d = (%+v, %v), want it completed with its outcome", published.Revision, settled, err)
		}

		next, _, err := store.Revise(t.Context(), ticket.ReviseInput{
			Ref: published, EventID: "event-after", By: in.RequestedBy,
			Draft: content("draft-2"), At: now.Add(time.Minute),
		})
		if err != nil {
			t.Fatalf("Revise after an answer: %v", err)
		}
		if next.Current.Phase != ticket.PhaseCollecting ||
			next.Current.Ref.Revision != published.Revision+1 {
			t.Fatalf("current = %+v, want a fresh revision collecting", next.Current)
		}
		if again, err := store.Revision(t.Context(), published); err != nil ||
			again.Phase != ticket.PhaseCompleted {
			t.Fatalf("the published revision was rewritten: (%+v, %v)", again, err)
		}
	})
}

/*
Closing is an act, and after it the ticket takes nothing.

Somebody says the request is done — in the flow this platform was written for,
by reacting to it — and from that moment a reply is a reply, not work.
*/
func TestStore_aClosedTicket_takesNoMoreCorrections(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		in := opening("event-closing")
		held, _, err := store.Open(t.Context(), in)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}

		closed, changed, err := store.CloseTicket(t.Context(), ticket.CloseTicketInput{
			Key: in.Key, By: "usr_manager", At: now.Add(time.Minute),
		})
		if err != nil || !changed || closed.Closed == nil {
			t.Fatalf("CloseTicket = (%+v, %v, %v)", closed, changed, err)
		}

		// Closing twice is not a conflict: the ticket is as closed as it was.
		if _, changed, err := store.CloseTicket(t.Context(), ticket.CloseTicketInput{
			Key: in.Key, By: "usr_other", At: now.Add(2 * time.Minute),
		}); err != nil || changed {
			t.Fatalf("second CloseTicket = (%v, %v), want it settled already", changed, err)
		}

		_, _, err = store.Revise(t.Context(), ticket.ReviseInput{
			Ref: held.Current.Ref, EventID: "event-late", By: in.RequestedBy,
			Draft: content("draft-late"), At: now.Add(3 * time.Minute),
		})
		if !errors.Is(err, ticket.ErrClosed) {
			t.Fatalf("Revise after closing = %v, want ErrClosed", err)
		}
	})
}

func publishOneAnswer(t *testing.T, store ticket.Store, held ticket.Ticket) (ref domain.TicketRef) {
	t.Helper()
	snapshot := content("snapshot-1")
	if _, _, err := store.RecordInspection(t.Context(), ticket.InspectionInput{
		Ref: held.Current.Ref, Snapshot: snapshot, At: now,
	}); err != nil {
		t.Fatalf("RecordInspection: %v", err)
	}
	if _, _, err := store.AwaitApproval(t.Context(), ticket.ApprovalInput{
		Ref: held.Current.Ref, RunID: "run-1", AtSeq: 4, Snapshot: snapshot, At: now,
	}); err != nil {
		t.Fatalf("AwaitApproval: %v", err)
	}
	claimed, _, err := store.ClaimExecution(t.Context(), ticket.ClaimInput{
		Ref: held.Current.Ref, RunID: "run-1", ApprovalAtSeq: 4, At: now,
	})
	if err != nil || claimed.Active == nil {
		t.Fatalf("ClaimExecution = (%+v, %v)", claimed, err)
	}
	if _, err := store.FinishExecution(t.Context(), ticket.FinishInput{
		Execution: *claimed.Active, Phase: ticket.PhaseCompleted,
		Result: content("answer-1"), At: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("FinishExecution: %v", err)
	}
	return held.Current.Ref
}
