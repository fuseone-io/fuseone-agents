package channel_test

import (
	"errors"
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

/*
The flow this admits: a form bot posts the request, and the team that owns it
is named later in the thread — by the triage bot, or by a person.

The mark is routing, never authority: it says which thread is a ticket, and
the ticket's identity stays the root, so every later reply lands on it.
*/
func TestTicketRoutes_aMarkedReplyUnderABotRoot_opensATicketKeyedOnTheRoot(t *testing.T) {
	_, channels, settingsStore := configuredChannelsWithStore(t)
	tickets := ticket.NewMemory()
	configureTicketRoom(t, channels)
	configureMarkedTicketRoom(t, channels)
	routes := channel.NewTicketRoutes(settingsStore, tickets)

	root, err := ticket.Key("acme-slack", "C-help", "171.1")
	if err != nil {
		t.Fatal(err)
	}
	base := channel.TicketCandidate{
		Connection: "acme-slack", Conversation: "C-help",
		Message: "171.2", Thread: "171.1", Kind: "message",
		Text:   ":large_yellow_circle: [team-sre] [RTD-17] Ticket criado",
		Source: channel.Source{User: "U-triage", Bot: "B-triage"},
	}
	for name, mutate := range map[string]func(*channel.TicketCandidate){
		"the configured triage bot": func(*channel.TicketCandidate) {},
		"a person in the channel": func(c *channel.TicketCandidate) {
			c.Message, c.Source = "171.3", channel.Source{User: "U-requester"}
			c.Text = "[team-sre]"
		},
		// Slack stamps the app id on what a person sends through an
		// integration. It is still that person writing.
		"a person writing through an app": func(c *channel.TicketCandidate) {
			c.Message = "171.4"
			c.Source = channel.Source{User: "U-requester", App: "A-assistant"}
			c.Text = "[team-sre]"
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base
			mutate(&candidate)
			intent, ok, err := routes.Route(t.Context(), candidate)
			if err != nil || !ok {
				t.Fatalf("Route: intent=%+v ok=%v err=%v", intent, ok, err)
			}
			if !intent.Root || !intent.Marked || intent.Key != root ||
				intent.RootFrom != "bot:B-forms" || intent.AddressedBy != "bot:B-triage" ||
				intent.Scope != (domain.Scope{Company: "acme", Area: "platform"}) ||
				intent.Agent != "ticketito" || intent.RunAs != "usr_platform" {
				t.Fatalf("intent = %+v, want the thread admitted on its root", intent)
			}
		})
	}

	for name, mutate := range map[string]func(*channel.TicketCandidate){
		"unmarked reply": func(c *channel.TicketCandidate) { c.Text = "alguém pode olhar?" },
		"a bot nobody trusted": func(c *channel.TicketCandidate) {
			c.Source = channel.Source{Bot: "B-stranger"}
		},
		"a bot posting under a person's name": func(c *channel.TicketCandidate) {
			c.Source = channel.Source{User: "U-bot-account", Bot: "B-stranger", App: "A-stranger"}
		},
		"the bot root itself": func(c *channel.TicketCandidate) {
			c.Message, c.Thread = "171.1", "171.1"
			c.Source = channel.Source{Bot: "B-forms"}
		},
		"a human root": func(c *channel.TicketCandidate) {
			c.Message, c.Thread = "171.5", "171.5"
			c.Source = channel.Source{User: "U-requester"}
		},
		"a mention": func(c *channel.TicketCandidate) { c.Kind = "mention" },
		"another room's pattern": func(c *channel.TicketCandidate) {
			c.Conversation, c.Text = "C-tickets", "please create an api key"
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base
			mutate(&candidate)
			if intent, ok, err := routes.Route(t.Context(), candidate); err != nil || ok {
				t.Fatalf("Route: intent=%+v ok=%v err=%v, want ignored", intent, ok, err)
			}
		})
	}
}

func configureMarkedTicketRoom(t *testing.T, channels *admin.Channels) {
	t.Helper()
	err := channels.PutConversation(t.Context(), "acme-slack", admin.Conversation{
		ID: "C-help", Label: "#dev-platform-help", Enabled: true,
		Scope: domain.Scope{Company: "acme", Area: "platform"},
		Mode:  channel.ConversationTicket, Agent: "ticketito", RunAs: "usr_platform",
		Ticket: &channel.TicketRule{
			OpenFrom: channel.TicketOpenMarkedThreads, RootFrom: "bot:B-forms",
			AddressFrom: "bot:B-triage", Patterns: []string{`\[team-sre\]`},
		},
	}, "usr_admin")
	if err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
}

func TestCompileTicketPatterns_aMarkedThreadRuleNeedsTheRootItTrusts(t *testing.T) {
	t.Parallel()
	valid := channel.TicketRule{
		OpenFrom: channel.TicketOpenMarkedThreads, RootFrom: "bot:B-forms",
		AddressFrom: "bot:B-triage", Patterns: []string{`\[team-sre\]`},
	}
	if _, err := channel.CompileTicketPatterns(valid); err != nil {
		t.Fatalf("CompileTicketPatterns: %v", err)
	}
	for name, mutate := range map[string]func(*channel.TicketRule){
		"no root source":   func(r *channel.TicketRule) { r.RootFrom = "" },
		"a person as root": func(r *channel.TicketRule) { r.RootFrom = "user:U1" },
		"a root on the older rule": func(r *channel.TicketRule) {
			r.OpenFrom = channel.TicketOpenLinkedUsers
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

/*
A refused rule says which field is wrong, and says so as the caller's fault.

One message for every cause sends somebody back to a form where four of the
five fields are already right — and a 500 tells them to call an operator about
a sentence they could have fixed themselves.
*/
func TestCompileTicketPatterns_eachMissingPieceIsNamedAndBlamedOnTheCaller(t *testing.T) {
	t.Parallel()
	complete := channel.TicketRule{
		OpenFrom: channel.TicketOpenMarkedThreads, RootFrom: "bot:B-forms",
		AddressFrom: "bot:B-triage", ReviewIn: "C-agents",
		Patterns: []string{`\[team-sre\]`},
	}
	if _, err := channel.CompileTicketPatterns(complete); err != nil {
		t.Fatalf("CompileTicketPatterns: %v", err)
	}

	for name, broken := range map[string]func(*channel.TicketRule){
		"no addressing source":   func(r *channel.TicketRule) { r.AddressFrom = "" },
		"a bare Slack id":        func(r *channel.TicketRule) { r.AddressFrom = "B-triage" },
		"no root source":         func(r *channel.TicketRule) { r.RootFrom = "" },
		"a root under the other": func(r *channel.TicketRule) { r.OpenFrom = channel.TicketOpenLinkedUsers },
		"an unknown policy":      func(r *channel.TicketRule) { r.OpenFrom = "whatever_comes_next" },
		"a room with a space":    func(r *channel.TicketRule) { r.ReviewIn = "C agents" },
		"no patterns":            func(r *channel.TicketRule) { r.Patterns = nil },
		"a pattern matching all": func(r *channel.TicketRule) { r.Patterns = []string{`.*`} },
	} {
		t.Run(name, func(t *testing.T) {
			rule := complete
			rule.Patterns = append([]string(nil), complete.Patterns...)
			broken(&rule)
			_, err := channel.CompileTicketPatterns(rule)
			if !errors.Is(err, channel.ErrTicketAdmission) {
				t.Fatalf("err = %v, want it refused as an admission rule", err)
			}
			if !admin.Invalid(err) {
				t.Fatalf("err = %v, want the console told the caller, not the operator", err)
			}
			if strings.TrimSpace(strings.TrimPrefix(err.Error(),
				channel.ErrTicketAdmission.Error()+":")) == "" {
				t.Fatalf("err = %v, want it to say which field", err)
			}
		})
	}
}
