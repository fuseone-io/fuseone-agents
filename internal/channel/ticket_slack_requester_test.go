package channel_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
)

/*
A room that accepts Slack requesters opens the ticket of somebody nobody bound
under their Slack account. It is who asked and never who decides: the run is
still the configured principal's.
*/
func TestTicketHandler_aMarkedThreadForAnUnboundAccount_opensUnderTheSlackAccount(t *testing.T) {
	handler, store, opener, _, threads := markedTicketHandler()
	threads.root.Text = "*[ autor(a) ]* <@USTRANGER> / Ana\nPreciso de ajuda"

	result := mustHandleTicket(t, handler, slackRequesterMark("event-mark"))
	if result.RunID == "" || opener.count() != 1 {
		t.Fatalf("Handle: result=%+v runs=%d", result, opener.count())
	}
	held, err := store.Current(t.Context(), markedTicketKey)
	if err != nil || held.RequestedBy != domain.SlackRequester("USTRANGER") || held.RunAs != "usr_platform" {
		t.Fatalf("ticket = %+v err=%v, want the Slack account asking under configured authority", held, err)
	}
	request := opener.last()
	if request.By != "usr_platform" || request.Ticket == nil ||
		request.Ticket.RequestedBy != domain.SlackRequester("USTRANGER") {
		t.Fatalf("request = %+v, want the run as the configured principal", request)
	}
}

func TestTicketHandler_aRoomThatAcceptsSlackRequesters_stillBindsWhoIsBound(t *testing.T) {
	handler, store, _, _, _ := markedTicketHandler()
	accounts := &ticketAccounts{name: "Pedro"}
	handler.WithAccounts(accounts)
	mustHandleTicket(t, handler, slackRequesterMark("event-mark"))

	held, _ := store.Current(t.Context(), markedTicketKey)
	if held.RequestedBy != "usr_requester" || len(accounts.asked) != 0 {
		t.Fatalf("requester = %q, profiles read = %v, want the bound person and no Slack lookup",
			held.RequestedBy, accounts.asked)
	}
}

// Slack's profile names the account, read once when the ticket opens so the
// agent can address somebody by name. A profile Slack will not give opens the
// ticket all the same: the name is a courtesy, not who asked.
func TestTicketHandler_aSlackRequesterIsNamedFromTheirProfile(t *testing.T) {
	for name, one := range map[string]struct {
		accounts *ticketAccounts
		want     string
	}{
		"named":      {&ticketAccounts{name: "Ana Souza"}, "Ana Souza"},
		"unreadable": {&ticketAccounts{err: errors.New("slack: refused: missing_scope")}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			handler, _, opener, _, threads := markedTicketHandler()
			threads.root.Text = "*[ autor(a) ]* <@USTRANGER> pediu isto"
			handler.WithAccounts(one.accounts)

			if result := mustHandleTicket(t, handler, slackRequesterMark("event-mark")); result.RunID == "" {
				t.Fatalf("result = %+v, want the ticket opened", result)
			}
			var input struct {
				Messages []struct {
					Name string `json:"name"`
				} `json:"messages"`
			}
			if err := json.Unmarshal(opener.last().Input, &input); err != nil ||
				len(input.Messages) != 1 || input.Messages[0].Name != one.want {
				t.Fatalf("input = %+v err=%v, want name %q", input, err, one.want)
			}
			if len(one.accounts.asked) != 1 || one.accounts.asked[0] != "acme-slack/USTRANGER" {
				t.Fatalf("profiles read = %v, want the account on its own connection", one.accounts.asked)
			}
		})
	}
}

type ticketAccounts struct {
	name  string
	err   error
	asked []string
}

func (a *ticketAccounts) Account(_ context.Context, connection, account string) (channel.Account, error) {
	a.asked = append(a.asked, connection+"/"+account)
	return channel.Account{Name: a.name}, a.err
}

func TestTicketHandler_aRootFromAnUnboundAccount_opensOnlyWhereTheRoomAcceptsIt(t *testing.T) {
	handler, _, opener, _ := ticketHandler()
	refused := ticketRoot("event-refused", "Please create an API key")
	refused.Source = channel.Source{User: "U-unbound"}
	if result := mustHandleTicket(t, handler, refused); result.Refusal.Reason != "ticket_unbound" {
		t.Fatalf("result = %+v, want the unbound account refused", result)
	}

	handler, store, opener, _ := ticketHandler()
	accepted := ticketRoot("event-accepted", "Please create an API key")
	accepted.Source = channel.Source{User: "U-unbound"}
	accepted.Ticket.SlackRequesters = true
	if result := mustHandleTicket(t, handler, accepted); result.RunID == "" || opener.count() != 1 {
		t.Fatalf("result = %+v runs=%d, want the ticket opened", result, opener.count())
	}
	held, _ := store.Current(t.Context(), channelTicketKey)
	if held.RequestedBy != domain.SlackRequester("U-unbound") {
		t.Fatalf("requester = %q, want the Slack account", held.RequestedBy)
	}
}

// The account that asked corrects its own ticket like a bound requester does;
// any other account nobody bound is a bystander.
func TestTicketHandler_anUnboundRequester_revisesOnlyTheirOwnTicket(t *testing.T) {
	handler, store, opener, _, threads := markedTicketHandler()
	threads.root.Text = "*[ autor(a) ]* <@USTRANGER> pediu isto"
	mustHandleTicket(t, handler, slackRequesterMark("event-mark"))

	other := markedReply("event-other", "171.3", channel.Source{User: "UOTHER"})
	if result := mustHandleTicket(t, handler, other); result.HandledReason != "ticket_author_ignored" {
		t.Fatalf("other account: result = %+v, want it ignored", result)
	}
	own := markedReply("event-own", "171.4", channel.Source{User: "USTRANGER"})
	if result := mustHandleTicket(t, handler, own); result.RunID == "" || opener.count() != 2 {
		t.Fatalf("own reply: result = %+v runs=%d", result, opener.count())
	}
	held, _ := store.Current(t.Context(), markedTicketKey)
	var input struct {
		Messages []struct {
			By string `json:"by"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(opener.last().Input, &input); err != nil || held.Current.Ref.Revision != 2 ||
		len(input.Messages) != 2 || input.Messages[1].By != "slack:USTRANGER" {
		t.Fatalf("revision=%d input=%+v err=%v, want the correction by the account",
			held.Current.Ref.Revision, input, err)
	}
}

func slackRequesterMark(event string) channel.Claimed {
	mark := markedTicketMark(event, "[team-sre]")
	mark.Ticket.SlackRequesters = true
	return mark
}

func markedReply(event, message string, source channel.Source) channel.Claimed {
	return channel.Claimed{Arrival: channel.Arrival{
		Channel: "acme-slack", Conversation: "C-help", EventID: event,
		Message: message, Thread: "171.1", Text: "É para produção", Source: source,
		Ticket: &channel.TicketIntent{Key: markedTicketKey},
	}}
}
