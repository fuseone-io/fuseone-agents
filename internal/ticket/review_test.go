package ticket_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/ticket"
)

/*
A ticket worked in a room of its own.

The room is configuration the ticket keeps, so a rule edited later does not
move a thread that is already open. The thread inside it is opened once, by
whichever worker claims the obligation, and after that the room's replies find
the ticket the same way the support thread's do.
*/
func TestStore_aTicketWithAReviewRoom_opensOneThreadAndIsFoundByIt(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		rooms, ok := store.(ticket.ReviewRooms)
		if !ok {
			t.Fatalf("%T does not lease review rooms", store)
		}
		in := opening("event-review")
		in.ReviewIn = "C-agents"
		opened, _, err := store.Open(t.Context(), in)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		if opened.Review.Conversation != "C-agents" || opened.Review.Root != "" {
			t.Fatalf("review = %+v, want the room configured and no thread yet", opened.Review)
		}

		owed, err := rooms.ClaimReviews(t.Context(), "worker-a", now, time.Minute, 10)
		if err != nil || len(owed) != 1 {
			t.Fatalf("ClaimReviews = (%+v, %v), want one thread owed", owed, err)
		}
		if owed[0].Key != in.Key || owed[0].Conversation != "C-agents" ||
			owed[0].Origin != in.Origin || owed[0].Scope != scope ||
			owed[0].RequestedBy != in.RequestedBy {
			t.Fatalf("notice = %+v", owed[0])
		}

		// Leased, so a second worker does not open a second thread.
		again, err := rooms.ClaimReviews(t.Context(), "worker-b", now, time.Minute, 10)
		if err != nil || len(again) != 0 {
			t.Fatalf("second claim = (%+v, %v), want none", again, err)
		}

		if err := rooms.MarkReviewOpened(t.Context(), in.Key, "900.1", "worker-a", now); err != nil {
			t.Fatalf("MarkReviewOpened: %v", err)
		}
		settled, err := rooms.ClaimReviews(t.Context(), "worker-a", now.Add(time.Hour), time.Minute, 10)
		if err != nil || len(settled) != 0 {
			t.Fatalf("claim after opening = (%+v, %v), want none", settled, err)
		}

		held, err := store.AtReview(t.Context(), ticket.Origin{
			Connection: in.Origin.Connection, Conversation: "C-agents", Root: "900.1",
		})
		if err != nil || held.Key != in.Key || held.Review.Root != "900.1" {
			t.Fatalf("AtReview = (%+v, %v), want the ticket its room is working", held, err)
		}
	})
}

func TestStore_aTicketWithNoReviewRoom_owesNoThreadAndIsNotFoundByOne(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ticket.Store) {
		rooms := store.(ticket.ReviewRooms)
		in := opening("event-no-review")
		if _, _, err := store.Open(t.Context(), in); err != nil {
			t.Fatalf("Open: %v", err)
		}
		owed, err := rooms.ClaimReviews(t.Context(), "worker-a", now, time.Minute, 10)
		if err != nil || len(owed) != 0 {
			t.Fatalf("ClaimReviews = (%+v, %v), want nothing owed", owed, err)
		}
		_, err = store.AtReview(t.Context(), ticket.Origin{
			Connection: in.Origin.Connection, Conversation: "C-agents", Root: "900.1",
		})
		if !errors.Is(err, ticket.ErrNotFound) {
			t.Fatalf("AtReview = %v, want ErrNotFound", err)
		}
	})
}
