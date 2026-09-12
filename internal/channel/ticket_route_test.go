package channel_test

import (
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ticket"
)

func TestCompileTicketPatterns_refusesAnAdmissionRuleItCannotEnforce(t *testing.T) {
	t.Parallel()
	valid := channel.TicketRule{
		OpenFrom: channel.TicketOpenLinkedUsers, AddressFrom: "bot:B-approvals",
		Patterns: []string{`(?i)api[ -]?key`},
	}
	for name, mutate := range map[string]func(*channel.TicketRule){
		"requester source": func(r *channel.TicketRule) { r.OpenFrom = "anyone" },
		"address source":   func(r *channel.TicketRule) { r.AddressFrom = "user:U1" },
		"missing pattern":  func(r *channel.TicketRule) { r.Patterns = nil },
		"invalid pattern":  func(r *channel.TicketRule) { r.Patterns = []string{"["} },
		"empty match":      func(r *channel.TicketRule) { r.Patterns = []string{".*"} },
		"large pattern": func(r *channel.TicketRule) {
			r.Patterns = []string{strings.Repeat("a", channel.MaxTicketPatternBytes+1)}
		},
		"many patterns": func(r *channel.TicketRule) {
			r.Patterns = make([]string, channel.MaxTicketPatterns+1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			rule := valid
			mutate(&rule)
			if _, err := channel.CompileTicketPatterns(rule); err == nil {
				t.Fatal("an admission rule this version cannot enforce was accepted")
			}
		})
	}
}

func TestTicketRoutes_onlyAMatchingHumanRoot_opensATicket(t *testing.T) {
	_, channels, settingsStore := configuredChannelsWithStore(t)
	tickets := ticket.NewMemory()
	configureTicketRoom(t, channels)
	routes := channel.NewTicketRoutes(settingsStore, tickets)

	base := channel.TicketCandidate{
		Connection: "acme-slack", Conversation: "C-tickets",
		Message: "171.1", Thread: "171.1", Kind: "message",
		Text: "Please create an API key", Source: channel.Source{User: "U-requester"},
	}
	intent, ok, err := routes.Route(t.Context(), base)
	if err != nil || !ok {
		t.Fatalf("Route: intent=%+v ok=%v err=%v", intent, ok, err)
	}
	if !intent.Root || intent.Scope != (domain.Scope{Company: "acme", Area: "support"}) ||
		intent.Agent != "gateway-support" || intent.RunAs != "usr_gateway" ||
		intent.AddressedBy != "bot:B-approvals" {
		t.Fatalf("intent = %+v, want the stored routing decision", intent)
	}

	for name, mutate := range map[string]func(*channel.TicketCandidate){
		"nonmatching": func(c *channel.TicketCandidate) { c.Text = "hello support" },
		"mention":     func(c *channel.TicketCandidate) { c.Kind = "mention" },
		"bot root": func(c *channel.TicketCandidate) {
			c.Source = channel.Source{User: "U-forged", Bot: "B-alert"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base
			mutate(&candidate)
			if _, ok, err := routes.Route(t.Context(), candidate); err != nil || ok {
				t.Fatalf("Route: ok=%v err=%v, want ignored", ok, err)
			}
		})
	}
}

func TestTicketRoutes_aReplyUsesTheStoredOrigin_notItsWords(t *testing.T) {
	_, _, settingsStore := configuredChannelsWithStore(t)
	tickets := ticket.NewMemory()
	key, err := ticket.Key("acme-slack", "C-tickets", "171.1")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = tickets.Open(t.Context(), ticket.OpenInput{
		Key: key, Origin: ticket.Origin{
			Connection: "acme-slack", Conversation: "C-tickets", Root: "171.1",
		},
		Scope:       domain.Scope{Company: "acme", Area: "support"},
		Agent:       "gateway-support",
		RunAs:       "usr_gateway",
		RequestedBy: "usr_requester", AddressedBy: "bot:B-approvals",
		EventID: "Ev-root", Draft: ticket.ContentRef{Ref: "content/root", Digest: "sha256:root"},
		At: time.Now(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	routes := channel.NewTicketRoutes(settingsStore, tickets)

	intent, ok, err := routes.Route(t.Context(), channel.TicketCandidate{
		Connection: "acme-slack", Conversation: "C-tickets",
		Message: "171.2", Thread: "171.1", Kind: "message",
		Text: "does not match any admission pattern", Source: channel.Source{App: "A-address"},
	})
	if err != nil || !ok || intent.Root || intent.Key != key {
		t.Fatalf("Route = %+v, %v, %v; want the existing ticket", intent, ok, err)
	}

	_, ok, err = routes.Route(t.Context(), channel.TicketCandidate{
		Connection: "other-slack", Conversation: "C-tickets",
		Message: "171.3", Thread: "171.1", Kind: "message",
	})
	if err != nil || ok {
		t.Fatalf("another connection routed to the ticket: ok=%v err=%v", ok, err)
	}
}

func TestSourceMatchesKey_usesTheActorTheRuleNamed(t *testing.T) {
	t.Parallel()
	source := channel.Source{Bot: "B-one", App: "A-one"}
	if !source.MatchesKey("bot:B-one") || !source.MatchesKey("app:A-one") {
		t.Fatal("the exact bot and app identities should both match")
	}
	if source.MatchesKey("bot:A-one") || source.MatchesKey("user:B-one") {
		t.Fatal("a different source kind matched by value alone")
	}
}

func configureTicketRoom(t *testing.T, channels *admin.Channels) {
	t.Helper()
	err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C-tickets", Label: "#api-key-support", Enabled: true,
		Scope: domain.Scope{Company: "acme", Area: "support"},
		Mode:  channel.ConversationTicket, Agent: "gateway-support", RunAs: "usr_gateway",
		Ticket: &channel.TicketRule{
			OpenFrom: channel.TicketOpenLinkedUsers, AddressFrom: "bot:B-approvals",
			Patterns: []string{`api[ -]?key`},
		},
	}, "usr_admin")
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
}
