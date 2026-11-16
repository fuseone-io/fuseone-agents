package channel_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/ticket"
)

/*
A conversation is reached only through a connection whose scope contains its
own (NT-019, phase 4 readers).

The door refuses such a conversation when it is written. These are the rows that
got past it anyway — a restore, a hand edit, a binary that did not check — and
the runtime is what holds against them. Each row is planted straight into the
store, bypassing the administration on purpose.
*/

var (
	cxVia       = channel.Via{Name: "cx-slack", Scope: domain.Scope{Company: "acme", Area: "cx"}}
	areaCX      = domain.Scope{Company: "acme", Area: "cx"}
	areaFinance = domain.Scope{Company: "acme", Area: "finance"}
	companyAcme = domain.Scope{Company: "acme"}
)

// A conversation of acme/finance on acme/cx's connection is posted nowhere:
// the area that owns the app would be told about another area's runs.
func TestFor_aConversationOutsideItsConnectionsScope_isNotPostedTo(t *testing.T) {
	store, _, settingsStore := configuredChannelsWithStore(t)
	connectionAt(t, settingsStore, "cx-slack", settings.ScopeArea, areaCX)
	conversationOn(t, settingsStore, "cx-slack", "C1-cx", settings.ScopeArea, areaCX, "announce")
	conversationOn(t, settingsStore, "cx-slack", "C2-fin", settings.ScopeArea, areaFinance, "announce")
	conversationOn(t, settingsStore, "cx-slack", "C3-acme", settings.ScopeCompany, companyAcme, "announce")

	if hears(t, store, "C2-fin", areaFinance) {
		t.Error("acme/finance's conversation on acme/cx's connection is posted to")
	}
	// A company's conversation hears every area of it, acme/cx included, but
	// the connection it names belongs to one area and cannot carry it.
	if hears(t, store, "C3-acme", areaCX) {
		t.Error("acme's conversation on acme/cx's connection is posted to")
	}
	if !hears(t, store, "C1-cx", areaCX) {
		t.Error("acme/cx's own conversation on its own connection is not posted to")
	}
}

// The installation's connection carries every scope's conversations, as it
// does today: nothing changes for a platform whose connections are its own.
func TestFor_theInstallationsConnection_carriesEveryScope(t *testing.T) {
	store, channels, settingsStore := configuredChannelsWithStore(t)
	connect(t, channels, "acme-slack")
	conversationOn(t, settingsStore, "acme-slack", "C4-fin", settings.ScopeArea, areaFinance, "announce")

	if !hears(t, store, "C4-fin", areaFinance) {
		t.Error("a conversation on the installation's connection is not posted to")
	}
}

// A connection at a scope this binary cannot judge carries nothing: read as
// the platform's, it would carry every area's conversations.
func TestFor_aConnectionAtAnUnknownScope_carriesNothing(t *testing.T) {
	store, _, settingsStore := configuredChannelsWithStore(t)
	connectionAt(t, settingsStore, "odd-slack", settings.ScopeArea,
		domain.Scope{Company: domain.Installation, Area: "cx"})
	conversationOn(t, settingsStore, "odd-slack", "C5-cx", settings.ScopeArea, areaCX, "announce")

	if hears(t, store, "C5-cx", areaCX) {
		t.Error("a conversation on a connection of no known scope is posted to")
	}
}

/*
Arrival holds the same fence, at every reader of an inbound message: an event
signed by acme/cx's app must not start, watch or route anything acme/finance
configured, even when a row says acme/finance's conversation is on that app.
*/
func TestArrival_aConversationOutsideItsConnectionsScope_isNotAdmitted(t *testing.T) {
	store, _, settingsStore := configuredChannelsWithStore(t)
	connectionAt(t, settingsStore, "cx-slack", settings.ScopeArea, areaCX)
	conversationOn(t, settingsStore, "cx-slack", "C6-fin", settings.ScopeArea, areaFinance, "mentions")
	conversationOn(t, settingsStore, "cx-slack", "C7-fin", settings.ScopeArea, areaFinance, "watch")

	if _, err := store.Resolve(t.Context(), cxVia, "C6-fin"); !errors.Is(err, channel.ErrNoConversation) {
		t.Errorf("Resolve = %v, want no conversation", err)
	}
	if threads, err := store.IncludeThreadContext(t.Context(), cxVia, "C6-fin"); err != nil || threads {
		t.Errorf("IncludeThreadContext = %v, %v; want false", threads, err)
	}
	_, ok, err := store.WatchFor(t.Context(), cxVia, "C7-fin", channel.Source{Bot: "B-alerts"})
	if err != nil || ok {
		t.Errorf("WatchFor = %v, %v; want no rule", ok, err)
	}
}

// The positive half, so the fence above is not passing by refusing
// everything: acme/cx's own conversation on acme/cx's own connection answers.
func TestArrival_aConversationInsideItsConnectionsScope_isAdmitted(t *testing.T) {
	store, _, settingsStore := configuredChannelsWithStore(t)
	connectionAt(t, settingsStore, "cx-slack", settings.ScopeArea, areaCX)
	conversationOn(t, settingsStore, "cx-slack", "C8-cx", settings.ScopeArea, areaCX, "mentions")
	conversationOn(t, settingsStore, "cx-slack", "C9-cx", settings.ScopeArea, areaCX, "watch")

	got, err := store.Resolve(t.Context(), cxVia, "C8-cx")
	if err != nil || got.Scope != areaCX {
		t.Errorf("Resolve = %+v, %v; want acme/cx", got, err)
	}
	if threads, err := store.IncludeThreadContext(t.Context(), cxVia, "C8-cx"); err != nil || !threads {
		t.Errorf("IncludeThreadContext = %v, %v; want true", threads, err)
	}
	_, ok, err := store.WatchFor(t.Context(), cxVia, "C9-cx", channel.Source{Bot: "B-alerts"})
	if err != nil || !ok {
		t.Errorf("WatchFor = %v, %v; want the rule", ok, err)
	}
}

// connectionAt plants a connection where the administration does not write
// one yet: a company's or an area's, or a scope no binary knows.
func connectionAt(
	t *testing.T, store *settings.Store, name string, kind settings.ScopeKind, scope domain.Scope,
) {
	t.Helper()
	if err := store.Put(t.Context(), settings.Setting{
		ScopeKind: kind, Scope: scope,
		Kind: channel.KindChannel, Name: name,
		Value: []byte(`{"kind":"slack"}`), Enabled: true, UpdatedBy: "restore",
	}); err != nil {
		t.Fatalf("write the connection %s: %v", name, err)
	}
}

// conversationOn plants a conversation on a connection, in the shape this
// version stores, at a scope of the test's choosing.
func conversationOn(
	t *testing.T, store *settings.Store, connection, id string,
	kind settings.ScopeKind, scope domain.Scope, mode string,
) {
	t.Helper()
	value := strings.Join([]string{
		`{"channel":"` + connection + `","keyVersion":2,"mode":"` + mode + `"`,
		`"agent":"triagem","runAs":"usr_opsbot","sources":["B-alerts"]`,
		`"threadContext":true}`,
	}, ",")
	if err := store.Put(t.Context(), settings.Setting{
		ScopeKind: kind, Scope: scope,
		Kind: channel.KindConversation, Name: channel.ConversationKey(connection, id),
		Value: []byte(value), Enabled: true, UpdatedBy: "restore",
	}); err != nil {
		t.Fatalf("write the conversation %s: %v", id, err)
	}
}

// A ticket opens only from a room its connection carries: acme/cx's app does
// not open acme/support's tickets, whatever the room's row says.
func TestTicketRoutes_aRoomOutsideItsConnectionsScope_opensNothing(t *testing.T) {
	_, channels, settingsStore := configuredChannelsWithStore(t)
	configureTicketRoom(t, channels)
	connectionAt(t, settingsStore, "acme-slack", settings.ScopeArea, areaCX)
	routes := channel.NewTicketRoutes(settingsStore, ticket.NewMemory())

	_, ok, err := routes.Route(t.Context(), ticketAsk(areaCX))
	if err != nil || ok {
		t.Fatalf("Route: ok=%v err=%v, want nothing opened", ok, err)
	}
}

// The positive half: the room's own area's app opens it.
func TestTicketRoutes_aRoomInsideItsConnectionsScope_opens(t *testing.T) {
	_, channels, settingsStore := configuredChannelsWithStore(t)
	configureTicketRoom(t, channels)
	connectionAt(t, settingsStore, "acme-slack", settings.ScopeArea,
		domain.Scope{Company: "acme", Area: "support"})
	routes := channel.NewTicketRoutes(settingsStore, ticket.NewMemory())

	if _, ok, err := routes.Route(t.Context(), ticketAsk(domain.Scope{Company: "acme", Area: "support"})); err != nil || !ok {
		t.Fatalf("Route: ok=%v err=%v, want the ticket opened", ok, err)
	}
}

func ticketAsk(received domain.Scope) channel.TicketCandidate {
	return channel.TicketCandidate{
		Connection: "acme-slack", ConnectionScope: received,
		Conversation: "C-tickets",
		Message:      "171.1", Thread: "171.1", Kind: "message",
		Text: "Please create an API key", Source: channel.Source{User: "U-requester"},
	}
}

/*
An event is judged by the scope of the session that received it, not by the
connection's row as it stands when the event is judged (NT-019, phase 4
readers).

A Socket Mode session is reconciled with its row every thirty seconds, and an
inbox row is consumed later still. In between the row can be removed, or
recreated under the same name at another scope. The event still came from the
app that held the old row; judged by the row instead, a removed connection
carried everything and a replaced one carried the newcomer's scope.
*/
func TestArrival_theConnectionRemovedBeforeTheEventIsJudged_isJudgedByTheSession(t *testing.T) {
	store, _, settingsStore := configuredChannelsWithStore(t)
	// No connection row at all: removed after the session opened.
	conversationOn(t, settingsStore, "cx-slack", "C10-fin", settings.ScopeArea, areaFinance, "mentions")

	if _, err := store.Resolve(t.Context(), cxVia, "C10-fin"); !errors.Is(err, channel.ErrNoConversation) {
		t.Errorf("Resolve = %v, want acme/cx's session to reach no acme/finance conversation", err)
	}
}

func TestArrival_theConnectionReplacedBeforeTheEventIsJudged_isJudgedByTheSession(t *testing.T) {
	store, _, settingsStore := configuredChannelsWithStore(t)
	// Recreated under the same name by acme/finance after acme/cx's session opened.
	connectionAt(t, settingsStore, "cx-slack", settings.ScopeArea, areaFinance)
	conversationOn(t, settingsStore, "cx-slack", "C11-fin", settings.ScopeArea, areaFinance, "watch")

	if _, ok, err := store.WatchFor(t.Context(), cxVia, "C11-fin", channel.Source{Bot: "B-alerts"}); err != nil || ok {
		t.Errorf("WatchFor = %v, %v; want acme/cx's session to reach nothing of acme/finance", ok, err)
	}
}

// A Via that names no scope is an arrival nobody said where it came from, and
// carries nothing: not even the installation's conversations.
func TestArrival_aViaWithNoScope_carriesNothing(t *testing.T) {
	store, channels, settingsStore := configuredChannelsWithStore(t)
	connect(t, channels, "acme-slack")
	conversationOn(t, settingsStore, "acme-slack", "C12-cx", settings.ScopeArea, areaCX, "mentions")

	if _, err := store.Resolve(t.Context(), channel.Via{Name: "acme-slack"}, "C12-cx"); !errors.Is(err, channel.ErrNoConversation) {
		t.Errorf("Resolve = %v, want nothing carried", err)
	}
}

// A ticket reply is judged by the session as well: removed or replaced, the
// row of the moment does not decide which tickets acme/support's app reaches.
func TestTicketRoutes_theConnectionRemovedOrReplaced_isJudgedByTheSession(t *testing.T) {
	support := domain.Scope{Company: "acme", Area: "support"}
	for name, after := range map[string]func(*testing.T, *settings.Store){
		"removed": func(t *testing.T, s *settings.Store) {
			if err := s.Delete(t.Context(), settings.ScopeArea, support, channel.KindChannel, "acme-slack"); err != nil {
				t.Fatalf("remove: %v", err)
			}
		},
		"replaced": func(t *testing.T, s *settings.Store) {
			if err := s.Delete(t.Context(), settings.ScopeArea, support, channel.KindChannel, "acme-slack"); err != nil {
				t.Fatalf("remove: %v", err)
			}
			connectionAt(t, s, "acme-slack", settings.ScopeArea, areaFinance)
		},
	} {
		t.Run(name, func(t *testing.T) {
			routes, settingsStore := financeTicketOnSupportSession(t)
			after(t, settingsStore)
			_, ok, err := routes.Route(t.Context(), channel.TicketCandidate{
				Connection: "acme-slack", ConnectionScope: support, Conversation: "C-tickets",
				Message: "171.2", Thread: "171.1", Kind: "message",
				Source: channel.Source{User: "U-requester"},
			})
			if err != nil || ok {
				t.Fatalf("Route: ok=%v err=%v, want acme/support's session to reach no acme/finance ticket", ok, err)
			}
		})
	}
}
