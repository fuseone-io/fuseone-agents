package channel

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/ticket"
)

// TicketCandidate is the bounded platform shape of one Slack message offered
// to ticket routing. Kind is "message" or "mention"; no vendor envelope is
// exposed past the Slack adapter.
type TicketCandidate struct {
	Connection   string
	Conversation string
	Message      string
	Thread       string
	Kind         string
	Text         string
	Source       Source
}

// TicketIntent is the routing decision persisted beside the inbox arrival.
// Root decisions carry the configuration snapshot that admitted them. Replies
// need only the durable key; the ticket itself already owns every identity.
type TicketIntent struct {
	Key         domain.TicketKey `json:"key"`
	Root        bool             `json:"root,omitempty"`
	Scope       domain.Scope     `json:"scope,omitempty"`
	Agent       domain.AgentID   `json:"agent,omitempty"`
	RunAs       domain.UserID    `json:"run_as,omitempty"`
	AddressedBy string           `json:"addressed_by,omitempty"`
	// Marked says the request is the thread's root and this arrival is only
	// the mark that admitted it. The root's text and its author are read at
	// handling time, where the connection can be asked.
	Marked   bool   `json:"marked,omitempty"`
	RootFrom string `json:"root_from,omitempty"`
	// ReviewIn travels with a decision that opens a ticket; Review says this
	// arrival came from the room rather than from the support thread.
	ReviewIn string `json:"review_in,omitempty"`
	Review   bool   `json:"review,omitempty"`
}

func (i TicketIntent) Valid() bool {
	if strings.TrimSpace(string(i.Key)) == "" {
		return false
	}
	if !i.Root {
		return i.Scope == (domain.Scope{}) && i.Agent == "" && i.RunAs == "" &&
			i.AddressedBy == "" && !i.Marked && i.RootFrom == "" && i.ReviewIn == ""
	}
	// A decision that opens a ticket names no room it came from: the room is
	// opened later, and nothing has replied in it yet.
	if i.Review {
		return false
	}
	// A marked decision carries the root it will trust, and only a marked one
	// does: a persisted intent that names a root source it would not check is
	// a decision nobody can read back.
	if i.Marked != (i.RootFrom != "") || (i.Marked && !addressSource(i.RootFrom)) {
		return false
	}
	return i.Scope.Valid() && i.Agent != "" && i.RunAs != "" &&
		addressSource(i.AddressedBy)
}

// TicketRouter is the one question a Slack door asks before considering an
// ordinary watch rule. The persisted answer is consumed later by the worker.
type TicketRouter interface {
	Route(context.Context, TicketCandidate) (TicketIntent, bool, error)
}

type compiledTicketRule struct {
	updatedAt   time.Time
	fingerprint [sha256.Size]byte
	patterns    []*regexp.Regexp
	rule        TicketRule
}

// TicketRoutes classifies roots from indexed configuration and replies from
// the ticket origin index. Compiled matchers are reused until the setting's
// update instant changes.
type TicketRoutes struct {
	settings *settings.Store
	tickets  ticket.Store
	log      *slog.Logger
	mu       sync.RWMutex
	compiled map[string]compiledTicketRule
}

// WithLog says where the routes explain themselves. Optional: without it they
// say the same things to the default logger.
func (r *TicketRoutes) WithLog(log *slog.Logger) *TicketRoutes {
	r.log = log
	return r
}

func (r *TicketRoutes) logger() *slog.Logger {
	if r.log == nil {
		return slog.Default()
	}
	return r.log
}

func NewTicketRoutes(settings *settings.Store, tickets ticket.Store) *TicketRoutes {
	return &TicketRoutes{
		settings: settings, tickets: tickets,
		compiled: make(map[string]compiledTicketRule),
	}
}

func (r *TicketRoutes) Route(
	ctx context.Context, candidate TicketCandidate,
) (TicketIntent, bool, error) {
	if candidate.Thread != candidate.Message {
		return r.reply(ctx, candidate)
	}
	if candidate.Kind != "message" || !candidate.Source.Person() ||
		len(candidate.Text) > MaxTicketMatchBytes || !utf8.ValidString(candidate.Text) {
		return TicketIntent{}, false, nil
	}
	return r.root(ctx, candidate)
}

func (r *TicketRoutes) reply(
	ctx context.Context, candidate TicketCandidate,
) (TicketIntent, bool, error) {
	if r.tickets == nil {
		return TicketIntent{}, false, nil
	}
	thread := ticket.Origin{
		Connection: candidate.Connection, Conversation: candidate.Conversation,
		Root: candidate.Thread,
	}
	held, err := r.tickets.AtOrigin(ctx, thread)
	if err == nil {
		return TicketIntent{Key: held.Key}, true, nil
	}
	if !errors.Is(err, ticket.ErrNotFound) {
		return TicketIntent{}, false, fmt.Errorf("channel: find ticket reply: %w", err)
	}
	// Not the support thread. It may still be the room where that ticket is
	// being written, which is a different index and the same durable identity.
	held, err = r.tickets.AtReview(ctx, thread)
	if err == nil {
		return TicketIntent{Key: held.Key, Review: true}, true, nil
	}
	if !errors.Is(err, ticket.ErrNotFound) {
		return TicketIntent{}, false, fmt.Errorf("channel: find ticket review reply: %w", err)
	}
	// No ticket on this thread at all. Under an ordinary rule that is the end
	// of it; under a marked one this reply may be what opens the thread.
	return r.mark(ctx, candidate)
}

func (r *TicketRoutes) root(
	ctx context.Context, candidate TicketCandidate,
) (TicketIntent, bool, error) {
	admitted, ok, err := r.admission(ctx, candidate)
	if err != nil || !ok {
		return TicketIntent{}, false, err
	}
	// A room whose tickets are opened by a mark is not a room where the root
	// is the request, whatever the root happens to say.
	if admitted.compiled.rule.OpenFrom != TicketOpenLinkedUsers ||
		!matchesTicket(admitted.compiled.patterns, candidate.Text) {
		return TicketIntent{}, false, nil
	}
	key, err := ticket.Key(candidate.Connection, candidate.Conversation, candidate.Message)
	if err != nil {
		return TicketIntent{}, false, err
	}
	return TicketIntent{
		Key: key, Root: true, Scope: admitted.scope,
		Agent: admitted.value.Agent, RunAs: admitted.value.RunAs,
		AddressedBy: admitted.compiled.rule.AddressFrom,
		ReviewIn:    admitted.compiled.rule.ReviewIn,
	}, true, nil
}

// ticketAdmission is one conversation's stored ticket rule, ready to decide.
type ticketAdmission struct {
	scope    domain.Scope
	value    conversationValue
	compiled compiledTicketRule
}

// admission answers with the rule configured for this conversation, if it has
// one at all. Not finding one is an answer, not a failure: most rooms have none.
func (r *TicketRoutes) admission(
	ctx context.Context, candidate TicketCandidate,
) (ticketAdmission, bool, error) {
	if r.settings == nil || r.tickets == nil {
		return ticketAdmission{}, false, nil
	}
	name := ConversationKey(candidate.Connection, candidate.Conversation)
	rows, err := r.settings.Named(ctx, KindConversation, name)
	if err != nil {
		return ticketAdmission{}, false, fmt.Errorf("channel: read ticket route: %w", err)
	}
	if len(rows) != 1 {
		if len(rows) > 1 {
			return ticketAdmission{}, false, ErrAmbiguousConversation
		}
		return ticketAdmission{}, false, nil
	}
	set := rows[0]
	var value conversationValue
	if err := json.Unmarshal(set.Value, &value); err != nil ||
		value.Channel != candidate.Connection || value.Mode != ConversationTicket || value.Ticket == nil {
		return ticketAdmission{}, false, nil
	}
	compiled, err := r.matcher(name, set.UpdatedAt, *value.Ticket)
	if err != nil {
		return ticketAdmission{}, false, fmt.Errorf("channel: compile stored ticket route: %w", err)
	}
	return ticketAdmission{scope: set.Scope, value: value, compiled: compiled}, true, nil
}
