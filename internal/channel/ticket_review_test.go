package channel_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/ticket"
)

/*
The room where the answer is written before anybody sees it.

A ticket configured with one is worked there: the people who can decide correct
the draft in that thread, each correction is a revision like any other, and the
support thread stays quiet until something is approved.
*/
func TestTicketHandler_aDeciderCorrectsTheDraftInTheReviewRoom(t *testing.T) {
	handler, store, opener, people, _ := markedTicketHandler()
	mustHandleTicket(t, handler, markedTicketMark("event-mark", "[team-sre]"))
	openReviewThread(t, store, "900.1")
	people.byAccount["UMANAGER"] = "usr_manager"

	result := mustHandleTicket(t, handler, reviewReply("event-review", "900.2",
		"pode retirar a parte do terraform", channel.Source{User: "UMANAGER"}))
	if result.RunID == "" || opener.count() != 2 {
		t.Fatalf("result = %+v, runs=%d, want the correction rewritten", result, opener.count())
	}

	held, _ := store.Current(t.Context(), markedTicketKey)
	if held.Current.Ref.Revision != 2 || held.RequestedBy != "usr_requester" {
		t.Fatalf("ticket = %+v, want a new revision and the same requester", held)
	}
	var input struct {
		Messages []struct {
			By   string `json:"by"`
			Text string `json:"text"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(opener.last().Input, &input); err != nil {
		t.Fatalf("input: %v", err)
	}
	if len(input.Messages) != 2 || input.Messages[1].By != "usr_manager" ||
		input.Messages[1].Text != "pode retirar a parte do terraform" {
		t.Fatalf("input = %+v, want the correction recorded under its author", input)
	}
}

func TestTicketHandler_theReviewRoomIgnoresWhoeverCannotDecide(t *testing.T) {
	for name, source := range map[string]channel.Source{
		"somebody with no say":    {User: "UREQUESTER"},
		"an account nobody bound": {User: "USTRANGER"},
		"the bot's own card":      {User: "U-triage", Bot: "B-triage"},
	} {
		t.Run(name, func(t *testing.T) {
			handler, store, opener, _, _ := markedTicketHandler()
			mustHandleTicket(t, handler, markedTicketMark("event-mark", "[team-sre]"))
			openReviewThread(t, store, "900.1")

			result := mustHandleTicket(t, handler,
				reviewReply("event-review", "900.2", "muda isso aqui", source))
			if result.RunID != "" || opener.count() != 1 {
				t.Fatalf("result = %+v, runs=%d, want nothing rewritten", result, opener.count())
			}
		})
	}
}

// Slack stamps the app id on what a person sends through an integration. The
// correction is theirs, and refusing it was how a whole afternoon of a room
// looked broken from the outside.
func TestTicketHandler_aCorrectionWrittenThroughAnApp_isStillTheirs(t *testing.T) {
	handler, store, opener, people, _ := markedTicketHandler()
	mustHandleTicket(t, handler, markedTicketMark("event-mark", "[team-sre]"))
	openReviewThread(t, store, "900.1")
	people.byAccount["UMANAGER"] = "usr_manager"

	result := mustHandleTicket(t, handler, reviewReply("event-review", "900.2",
		"tira a parte do terraform", channel.Source{User: "UMANAGER", App: "A-assistant"}))
	if result.RunID == "" || opener.count() != 2 {
		t.Fatalf("result = %+v, runs=%d, want the correction taken", result, opener.count())
	}
}

// The room's thread is opened once, and what the driver answers is what later
// cards are posted under.
func TestTicketReviewConsumer_opensOneThreadPerTicketAndRecordsWhereItIs(t *testing.T) {
	handler, store, _, _, _ := markedTicketHandler()
	mustHandleTicket(t, handler, markedTicketMark("event-mark", "[team-sre]"))
	rooms := &reviewOpenings{ref: "900.7"}
	consumer := channel.NewTicketReviewConsumer(store, rooms, "worker-a")

	opened, err := consumer.Sweep(t.Context(), time.Minute, 10)
	if err != nil || opened != 1 {
		t.Fatalf("Sweep = (%d, %v), want one room opened", opened, err)
	}
	if len(rooms.asked) != 1 || rooms.asked[0].Conversation != "C-agents" ||
		rooms.asked[0].From != "C-help" || rooms.asked[0].Root != "171.1" {
		t.Fatalf("openings = %+v, want the room told which request it is", rooms.asked)
	}
	held, _ := store.Current(t.Context(), markedTicketKey)
	if held.Review.Root != "900.7" {
		t.Fatalf("review = %+v, want the thread recorded", held.Review)
	}

	// Recorded, so a second pass has nothing to open and posts nothing.
	again, err := consumer.Sweep(t.Context(), time.Minute, 10)
	if err != nil || again != 0 || len(rooms.asked) != 1 {
		t.Fatalf("second sweep = (%d, %v), openings=%d", again, err, len(rooms.asked))
	}
}

func TestTicketReviewConsumer_aRoomThatCannotBePostedIn_keepsTheTicketOwed(t *testing.T) {
	handler, store, _, _, _ := markedTicketHandler()
	mustHandleTicket(t, handler, markedTicketMark("event-mark", "[team-sre]"))
	rooms := &reviewOpenings{err: errors.New("slack: not_in_channel")}
	consumer := channel.NewTicketReviewConsumer(store, rooms, "worker-a")

	if opened, err := consumer.Sweep(t.Context(), -time.Second, 10); err == nil || opened != 0 {
		t.Fatalf("Sweep = (%d, %v), want the failure reported", opened, err)
	}
	held, _ := store.Current(t.Context(), markedTicketKey)
	if held.Review.Root != "" {
		t.Fatalf("review = %+v, want no thread recorded", held.Review)
	}
	rooms.err, rooms.ref = nil, "900.9"
	if opened, err := consumer.Sweep(t.Context(), time.Minute, 10); err != nil || opened != 1 {
		t.Fatalf("retry = (%d, %v), want the room opened once Slack answers", opened, err)
	}
}

type reviewOpenings struct {
	ref   string
	err   error
	asked []channel.ReviewOpening
}

func (r *reviewOpenings) OpenReview(
	_ context.Context, _ string, opening channel.ReviewOpening,
) (string, error) {
	r.asked = append(r.asked, opening)
	if r.err != nil {
		return "", r.err
	}
	return r.ref, nil
}

func openReviewThread(t *testing.T, store *ticket.Memory, root string) {
	t.Helper()
	owed, err := store.ClaimReviews(t.Context(), "worker-test", ticketNow, time.Minute, 10)
	if err != nil || len(owed) != 1 {
		t.Fatalf("ClaimReviews = (%+v, %v), want the room owed one thread", owed, err)
	}
	if err := store.MarkReviewOpened(t.Context(), owed[0].Key, root, "worker-test", ticketNow); err != nil {
		t.Fatalf("MarkReviewOpened: %v", err)
	}
}

func reviewReply(event, message, text string, source channel.Source) channel.Claimed {
	return channel.Claimed{Arrival: channel.Arrival{
		Channel: "acme-slack", Conversation: "C-agents", EventID: event,
		Message: message, Thread: "900.1", Text: text, Source: source,
		Ticket: &channel.TicketIntent{Key: markedTicketKey, Review: true},
	}}
}

/*
Nothing about a ticket is said where the person who asked is reading.

A refusal, a notice, a card: all of it belongs in the room the ticket is worked
in. The support thread hears one thing, once — the answer somebody approved.
This one is the refusal that started it: a thread marked for a requester nobody
has bound, which used to be answered under the request itself.
*/
func TestConsumer_aTicketRefusal_isSaidInTheReviewRoomAndNotInTheThread(t *testing.T) {
	handler, _, _, _, threads := markedTicketHandler()
	threads.root.Text = "*[ autor(a) ]* <@USTRANGER> pediu isto"

	mark := markedTicketMark("event-mark", "[team-sre]")
	consumer, parts := consumerWith(t, mark.Text, func(parts *consumerParts) {
		parts.arrival = mark.Arrival
		parts.arrival.Payload = []byte(`{}`)
	})
	consumer.WithTickets(handler)

	if _, err := consumer.Sweep(t.Context(), time.Minute, 5); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	said, err := consumer.Answer(t.Context(), time.Minute, 5)
	if err != nil || said != 1 {
		t.Fatalf("Answer = (%d, %v), want the refusal delivered", said, err)
	}
	if len(parts.answers.to) != 1 || parts.answers.to[0] != "C-agents/" {
		t.Fatalf("said in %v, want the review room", parts.answers.to)
	}
}

/*
The room is where work is asked for; the support thread is where it was asked.

With a room open, the person who asked can still add to their own request — a
detail they forgot, an answer to a question — and none of it starts a run. What
regenerates the answer is a correction in the room, which is the only place
somebody is deciding.
*/
func TestTicketHandler_withAReviewRoom_theRequestersReply_isKeptWithoutRunning(t *testing.T) {
	handler, store, opener, people, _ := markedTicketHandler()
	mustHandleTicket(t, handler, markedTicketMark("event-mark", "[team-sre]"))
	openReviewThread(t, store, "900.1")
	people.byAccount["UMANAGER"] = "usr_manager"

	reply := channel.Claimed{Arrival: channel.Arrival{
		Channel: "acme-slack", Conversation: "C-help", EventID: "event-more",
		Message: "171.3", Thread: "171.1", Text: "é para o ambiente de produção",
		Source: channel.Source{User: "UREQUESTER"},
		Ticket: &channel.TicketIntent{Key: markedTicketKey},
	}}
	result := mustHandleTicket(t, handler, reply)
	if result.RunID != "" || opener.count() != 1 {
		t.Fatalf("result = %+v, runs=%d, want nothing started", result, opener.count())
	}

	held, _ := store.Current(t.Context(), markedTicketKey)
	if held.Current.Ref.Revision != 2 {
		t.Fatalf("revision = %d, want the detail kept", held.Current.Ref.Revision)
	}

	// And the room still drives: a correction there regenerates from the
	// request as it now stands.
	correction := reviewReply("event-correct", "900.2", "tira a parte do terraform",
		channel.Source{User: "UMANAGER"})
	if result := mustHandleTicket(t, handler, correction); result.RunID == "" || opener.count() != 2 {
		t.Fatalf("correction = %+v, runs=%d", result, opener.count())
	}
}

// Nothing is folded in while a decision is pending: a message arriving then
// would supersede the card somebody is reading, without anything regenerating.
func TestTicketHandler_withADecisionPending_theRequestersReply_changesNothing(t *testing.T) {
	handler, store, opener, _, _ := markedTicketHandler()
	mustHandleTicket(t, handler, markedTicketMark("event-mark", "[team-sre]"))
	openReviewThread(t, store, "900.1")

	held, _ := store.Current(t.Context(), markedTicketKey)
	snapshot := ticket.ContentRef{Ref: "content://snapshot", Digest: "sha256:snapshot"}
	if _, _, err := store.RecordInspection(t.Context(), ticket.InspectionInput{
		Ref: held.Current.Ref, Snapshot: snapshot, At: ticketNow,
	}); err != nil {
		t.Fatalf("RecordInspection: %v", err)
	}
	if _, _, err := store.AwaitApproval(t.Context(), ticket.ApprovalInput{
		Ref: held.Current.Ref, RunID: "run-1", AtSeq: 4, Snapshot: snapshot, At: ticketNow,
	}); err != nil {
		t.Fatalf("AwaitApproval: %v", err)
	}

	reply := channel.Claimed{Arrival: channel.Arrival{
		Channel: "acme-slack", Conversation: "C-help", EventID: "event-late",
		Message: "171.4", Thread: "171.1", Text: "obrigado!",
		Source: channel.Source{User: "UREQUESTER"},
		Ticket: &channel.TicketIntent{Key: markedTicketKey},
	}}
	if result := mustHandleTicket(t, handler, reply); result.RunID != "" || opener.count() != 1 {
		t.Fatalf("result = %+v, runs=%d", result, opener.count())
	}
	after, _ := store.Current(t.Context(), markedTicketKey)
	if after.Current.Ref.Revision != held.Current.Ref.Revision {
		t.Fatalf("revision = %d, want the pending decision untouched", after.Current.Ref.Revision)
	}
}

/*
A reaction ends the request, and only from somebody who may decide.

The emoji is the room's own — configured beside the patterns that open one —
and it has to be on the request itself. Anything else is a person reacting to a
message, which is what people do all day.
*/
func TestTicketHandler_aClosingReaction_endsTheTicket(t *testing.T) {
	handler, store, opener, people, _ := markedTicketHandler()
	mustHandleTicket(t, handler, markedTicketMark("event-mark", "[team-sre]"))
	openReviewThread(t, store, "900.1")
	people.byAccount["UMANAGER"] = "usr_manager"

	// Somebody who cannot decide reacts: nothing happens, and nothing is said.
	stranger := closingReaction("event-react-stranger", "UREQUESTER")
	if result := mustHandleTicket(t, handler, stranger); result.HandledReason != "ticket_closer_ignored" {
		t.Fatalf("stranger = %+v, want it ignored", result)
	}
	if held, _ := store.Current(t.Context(), markedTicketKey); held.Closed != nil {
		t.Fatal("a reaction from somebody who cannot decide closed the ticket")
	}

	if result := mustHandleTicket(t, handler, closingReaction("event-react", "UMANAGER")); result.HandledReason != "ticket_closed" {
		t.Fatalf("closing = %+v", result)
	}
	held, _ := store.Current(t.Context(), markedTicketKey)
	if held.Closed == nil || held.ClosedBy != "usr_manager" {
		t.Fatalf("ticket = %+v, want it closed by the person who reacted", held)
	}

	// And nothing works it afterwards.
	late := reviewReply("event-late", "900.9", "mais uma correção",
		channel.Source{User: "UMANAGER"})
	if result := mustHandleTicket(t, handler, late); result.RunID != "" || opener.count() != 1 {
		t.Fatalf("late correction = %+v, runs=%d", result, opener.count())
	}
}

func closingReaction(event, user string) channel.Claimed {
	return channel.Claimed{Arrival: channel.Arrival{
		Channel: "acme-slack", Conversation: "C-help", EventID: event,
		Message: "171.1", Thread: "171.1",
		Source: channel.Source{User: user},
		Ticket: &channel.TicketIntent{Key: markedTicketKey, Close: true},
	}}
}
