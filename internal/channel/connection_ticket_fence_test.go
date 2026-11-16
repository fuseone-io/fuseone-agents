package channel_test

import (
	"testing"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/ticket"
)

/*
A ticket already open is reached through a connection only inside the
connection's scope (NT-019, phase 4 readers).

A durable ticket outlives the connection it was opened on: removing a
connection does not remove its tickets, and a connection recreated under the
same name, by another area, inherits every origin they name. Its owner holds
the new app's signing secret and can forge those origins' identifiers. So a
reply, a reply in the review room and a closing reaction each judge the ticket's
own scope against the connection's, not only the room's.

Here the room on acme-slack is acme/support's, and so is the connection; the
ticket at its origin is acme/finance's, as one left by the connection that held
the name before.
*/
func TestTicketRoutes_aReplyToATicketOutsideItsConnectionsScope_reachesNothing(t *testing.T) {
	routes, _ := reusedNameTickets(t)

	_, ok, err := routes.Route(t.Context(), channel.TicketCandidate{
		Connection: "acme-slack", Conversation: "C-tickets",
		Message: "171.2", Thread: "171.1", Kind: "message",
		Source: channel.Source{User: "U-requester"},
	})
	if err != nil || ok {
		t.Fatalf("Route: ok=%v err=%v, want the reply to reach no ticket", ok, err)
	}
}

func TestTicketRoutes_aReviewReplyToATicketOutsideItsConnectionsScope_reachesNothing(t *testing.T) {
	routes, tickets := reusedNameTickets(t)
	openReviewThread(t, tickets, "900.1")

	_, ok, err := routes.Route(t.Context(), channel.TicketCandidate{
		Connection: "acme-slack", Conversation: "C-review",
		Message: "900.2", Thread: "900.1", Kind: "message",
		Source: channel.Source{User: "U-decider"},
	})
	if err != nil || ok {
		t.Fatalf("Route: ok=%v err=%v, want the review reply to reach no ticket", ok, err)
	}
}

func TestTicketRoutes_aReactionOnATicketOutsideItsConnectionsScope_closesNothing(t *testing.T) {
	routes, _ := reusedNameTickets(t)

	_, ok, err := routes.Route(t.Context(), channel.TicketCandidate{
		Connection: "acme-slack", Conversation: "C-tickets",
		Message: "171.1", Thread: "171.1", Kind: "reaction", Reaction: "white_check_mark",
		Source: channel.Source{User: "U-decider"},
	})
	if err != nil || ok {
		t.Fatalf("Route: ok=%v err=%v, want the reaction to close nothing", ok, err)
	}
}

// The positive half: the same three moves on a ticket of the connection's own
// scope reach it, so the refusals above are the fence and not a route that
// stopped answering.
func TestTicketRoutes_aTicketInsideItsConnectionsScope_isReached(t *testing.T) {
	support := domain.Scope{Company: "acme", Area: "support"}
	for name, candidate := range map[string]channel.TicketCandidate{
		"reply": {Connection: "acme-slack", Conversation: "C-tickets",
			Message: "171.2", Thread: "171.1", Kind: "message", Source: channel.Source{User: "U-requester"}},
		"review": {Connection: "acme-slack", Conversation: "C-review",
			Message: "900.2", Thread: "900.1", Kind: "message", Source: channel.Source{User: "U-decider"}},
		"reaction": {Connection: "acme-slack", Conversation: "C-tickets",
			Message: "171.1", Thread: "171.1", Kind: "reaction", Reaction: "white_check_mark",
			Source: channel.Source{User: "U-decider"}},
	} {
		t.Run(name, func(t *testing.T) {
			routes, tickets := ticketsOpenedAt(t, support)
			openReviewThread(t, tickets, "900.1")
			if _, ok, err := routes.Route(t.Context(), candidate); err != nil || !ok {
				t.Fatalf("Route: ok=%v err=%v, want the ticket reached", ok, err)
			}
		})
	}
}

func reusedNameTickets(t *testing.T) (*channel.TicketRoutes, *ticket.Memory) {
	t.Helper()
	return ticketsOpenedAt(t, domain.Scope{Company: "acme", Area: "finance"})
}

// ticketsOpenedAt is acme/support's room on acme/support's own connection,
// holding one open ticket stored at the scope given.
func ticketsOpenedAt(t *testing.T, scope domain.Scope) (*channel.TicketRoutes, *ticket.Memory) {
	t.Helper()
	_, channels, settingsStore := configuredChannelsWithStore(t)
	support := domain.Scope{Company: "acme", Area: "support"}
	connectionAt(t, settingsStore, "acme-slack", settings.ScopeArea, support)
	if err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C-tickets", Enabled: true, Scope: support,
		Mode: channel.ConversationTicket, Agent: "gateway-support", RunAs: "usr_gateway",
		Ticket: &channel.TicketRule{
			OpenFrom: channel.TicketOpenLinkedUsers, AddressFrom: "bot:B-approvals",
			Patterns: []string{`api[ -]?key`}, ClosesOn: []string{"white_check_mark"},
		},
	}, "usr_admin"); err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
	tickets := ticket.NewMemory()
	key, err := ticket.Key("acme-slack", "C-tickets", "171.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := tickets.Open(t.Context(), ticket.OpenInput{
		Key: key, Origin: ticket.Origin{
			Connection: "acme-slack", Conversation: "C-tickets", Root: "171.1",
		},
		Scope: scope, Agent: "gateway-support", RunAs: "usr_gateway",
		RequestedBy: "usr_requester", AddressedBy: "bot:B-approvals", ReviewIn: "C-review",
		EventID: "Ev-root", Draft: ticket.ContentRef{Ref: "content/root", Digest: "sha256:root"},
		At: ticketNow.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	return channel.NewTicketRoutes(settingsStore, tickets), tickets
}
