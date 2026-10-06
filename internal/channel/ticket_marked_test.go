package channel_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/ticket"
)

var markedTicketKey = func() domain.TicketKey {
	key, err := ticket.Key("acme-slack", "C-help", "171.1")
	if err != nil {
		panic(err)
	}
	return key
}()

// The form bot's root, as the help channel actually writes one: the person who
// asked is named inside the text, because the message is not theirs.
const markedRootText = "*[ autor(a) ]* <@UREQUESTER|Pedro> / Pedro\n" +
	"*[ descrição ]*\nPreciso de ajuda para criar um monitor"

func TestTicketHandler_aMarkedThreadOpensTheRequestWrittenInItsRoot(t *testing.T) {
	handler, store, opener, _, threads := markedTicketHandler()
	result := mustHandleTicket(t, handler, markedTicketMark("event-mark", "[team-sre] [RTD-17]"))
	if result.RunID == "" || opener.count() != 1 {
		t.Fatalf("Handle: result=%+v runs=%d", result, opener.count())
	}
	if len(threads.asked) != 1 || threads.asked[0] != "171.1" {
		t.Fatalf("threads read = %v, want the marked thread's root", threads.asked)
	}

	held, err := store.Current(t.Context(), markedTicketKey)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if held.RequestedBy != "usr_requester" || held.Origin.Root != "171.1" ||
		held.RunAs != "usr_platform" || held.Agent != "ticketito" {
		t.Fatalf("ticket = %+v, want the root's author asking under configured authority", held)
	}

	request := opener.last()
	if request.Origin == nil || request.Origin.Thread != "171.1" ||
		request.Ticket == nil || request.Ticket.RequestedBy != "usr_requester" {
		t.Fatalf("request = %+v, want the run addressed at the root", request)
	}
	var input struct {
		Messages []struct {
			Ref  string `json:"ref"`
			By   string `json:"by"`
			Text string `json:"text"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(request.Input, &input); err != nil {
		t.Fatalf("input: %v", err)
	}
	// The mark says which thread is a ticket. What the ticket asks for is the
	// root: an agent given "[team-sre]" alone has been told nothing.
	if len(input.Messages) != 1 || input.Messages[0].Ref != "171.1" ||
		input.Messages[0].By != "usr_requester" || input.Messages[0].Text != markedRootText {
		t.Fatalf("input = %+v, want the root as the request", input)
	}
}

func TestTicketHandler_aMarkedThreadUnderAnUntrustedRoot_opensNothing(t *testing.T) {
	handler, store, opener, _, threads := markedTicketHandler()
	threads.root.Source = "bot:B-stranger"

	result := mustHandleTicket(t, handler, markedTicketMark("event-mark", "[team-sre]"))
	if result.HandledReason != "ticket_root_not_admitted" || opener.count() != 0 {
		t.Fatalf("result = %+v, runs=%d, want the thread left alone", result, opener.count())
	}
	if _, err := store.Current(t.Context(), markedTicketKey); !errors.Is(err, ticket.ErrNotFound) {
		t.Fatalf("Current = %v, want no ticket", err)
	}
}

func TestTicketHandler_aMarkedThreadWhoseRootIsGone_opensNothing(t *testing.T) {
	handler, _, opener, _, threads := markedTicketHandler()
	threads.root.Ref = "171.0"

	result := mustHandleTicket(t, handler, markedTicketMark("event-mark", "[team-sre]"))
	if result.HandledReason != "ticket_root_unreadable" || opener.count() != 0 {
		t.Fatalf("result = %+v, runs=%d", result, opener.count())
	}
}

// Two refusals, because they are two things to go and fix: an account nobody
// has bound, and a root that names no account at all.
func TestTicketHandler_aMarkedThreadWithNoRequesterToBind_saysWhichProblemItIs(t *testing.T) {
	for name, one := range map[string]struct{ root, reason string }{
		"an account nobody bound": {
			"*[ autor(a) ]* <@USTRANGER> pediu isto", "ticket_unbound",
		},
		"nobody named in the root": {
			"*[ descrição ]*\nPreciso de ajuda", "ticket_requester_unnamed",
		},
		"a mention that is a group": {
			"*[ autor(a) ]* <!subteam^S-sre> pediu isto", "ticket_requester_unnamed",
		},
		"a mention only below the first line": {
			"*[ descrição ]*\nvejam com <@UREQUESTER>", "ticket_requester_unnamed",
		},
	} {
		t.Run(name, func(t *testing.T) {
			handler, _, opener, _, threads := markedTicketHandler()
			threads.root.Text = one.root

			result := mustHandleTicket(t, handler, markedTicketMark("event-mark", "[team-sre]"))
			if result.Refusal.Reason != one.reason || opener.count() != 0 {
				t.Fatalf("result = %+v, runs=%d, want %s", result, opener.count(), one.reason)
			}
		})
	}
}

func TestTicketHandler_aMarkedThreadWhoseConnectionIsDown_isRetried(t *testing.T) {
	handler, _, opener, _, threads := markedTicketHandler()
	threads.err = errors.New("slack: refused: ratelimited")

	_, err := handler.Handle(t.Context(), markedTicketMark("event-mark", "[team-sre]"))
	if err == nil || opener.count() != 0 {
		t.Fatalf("Handle = %v, runs=%d, want the arrival left to retry", err, opener.count())
	}
}

func TestTicketHandler_aMarkedThreadWithNoThreadReader_failsClosed(t *testing.T) {
	store := ticket.NewMemory()
	people := &ticketPeople{byAccount: map[string]domain.UserID{"UREQUESTER": "usr_requester"}}
	handler := channel.NewTicketHandler(
		store, engine.NewMemoryContent(), &ticketOpener{}, people, people,
		ticketDeciders{"usr_manager"}, ledger.NewMemory(), func() time.Time { return ticketNow },
	)

	_, err := handler.Handle(t.Context(), markedTicketMark("event-mark", "[team-sre]"))
	if !errors.Is(err, channel.ErrNotWired) {
		t.Fatalf("Handle = %v, want ErrNotWired", err)
	}
}

// After the mark, the thread behaves like any other ticket: the person the
// root named is its requester, and their replies are corrections to it.
func TestTicketHandler_afterAMark_theRootsAuthorRevisesTheTicket(t *testing.T) {
	handler, store, opener, _, _ := markedTicketHandler()
	mustHandleTicket(t, handler, markedTicketMark("event-mark", "[team-sre]"))

	reply := channel.Claimed{Arrival: channel.Arrival{
		Channel: "acme-slack", Conversation: "C-help", EventID: "event-reply",
		Message: "171.3", Thread: "171.1", Text: "É para o ambiente de produção",
		Source: channel.Source{User: "UREQUESTER"},
		Ticket: &channel.TicketIntent{Key: markedTicketKey},
	}}
	if result := mustHandleTicket(t, handler, reply); result.RunID == "" || opener.count() != 2 {
		t.Fatalf("reply result = %+v, runs=%d", result, opener.count())
	}
	held, _ := store.Current(t.Context(), markedTicketKey)
	if held.Current.Ref.Revision != 2 {
		t.Fatalf("revision = %d, want the correction folded into the ticket", held.Current.Ref.Revision)
	}
}

type ticketThreads struct {
	root  channel.ThreadMessage
	err   error
	asked []string
}

func (r *ticketThreads) Thread(
	_ context.Context, _, _, thread, _ string,
) (channel.ThreadContext, error) {
	r.asked = append(r.asked, thread)
	if r.err != nil {
		return channel.ThreadContext{}, r.err
	}
	return channel.ThreadContext{
		Thread:   thread,
		Messages: []channel.ThreadMessage{r.root, {Ref: "171.2", Source: "bot:B-triage", Text: "[team-sre]"}},
	}, nil
}

func markedTicketHandler() (
	*channel.TicketHandler, *ticket.Memory, *ticketOpener, *ticketPeople, *ticketThreads,
) {
	store := ticket.NewMemory()
	opener := &ticketOpener{}
	people := &ticketPeople{byAccount: map[string]domain.UserID{"UREQUESTER": "usr_requester"}}
	threads := &ticketThreads{root: channel.ThreadMessage{
		Ref: "171.1", Source: "bot:B-forms", Text: markedRootText,
	}}
	handler := channel.NewTicketHandler(
		store, engine.NewMemoryContent(), opener, people, people,
		ticketDeciders{"usr_manager"}, ledger.NewMemory(), func() time.Time { return ticketNow },
	).WithThreads(threads)
	return handler, store, opener, people, threads
}

func markedTicketMark(event, text string) channel.Claimed {
	return channel.Claimed{Arrival: channel.Arrival{
		Channel: "acme-slack", Conversation: "C-help", EventID: event,
		Message: "171.2", Thread: "171.1", Text: text,
		Source: channel.Source{User: "U-triage", Bot: "B-triage"},
		Ticket: &channel.TicketIntent{
			Key: markedTicketKey, Root: true, Marked: true,
			Scope: domain.Scope{Company: "acme", Area: "platform"},
			Agent: "ticketito", RunAs: "usr_platform",
			AddressedBy: "bot:B-triage", RootFrom: "bot:B-forms", ReviewIn: "C-agents",
		},
	}}
}
