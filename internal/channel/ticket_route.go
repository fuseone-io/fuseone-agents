package channel

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/ticket"
)

const (
	TicketOpenLinkedUsers  = "linked_users"
	MaxTicketPatterns      = 8
	MaxTicketPatternBytes  = 256
	MaxTicketMatchBytes    = 8 * 1024
	maxCompiledTicketRules = 512
)

// TicketRule is the admission policy for root messages in one conversation.
// The first release deliberately admits only linked human roots and one exact
// bot/app addressing source.
type TicketRule struct {
	OpenFrom    string   `json:"openFrom"`
	AddressFrom string   `json:"addressFrom"`
	Patterns    []string `json:"patterns"`
}

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
}

func (i TicketIntent) Valid() bool {
	if strings.TrimSpace(string(i.Key)) == "" {
		return false
	}
	if !i.Root {
		return i.Scope == (domain.Scope{}) && i.Agent == "" && i.RunAs == "" &&
			i.AddressedBy == ""
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
	mu       sync.RWMutex
	compiled map[string]compiledTicketRule
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
	if candidate.Kind != "message" || candidate.Source.User == "" ||
		candidate.Source.Bot != "" || candidate.Source.App != "" ||
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
	held, err := r.tickets.AtOrigin(ctx, ticket.Origin{
		Connection: candidate.Connection, Conversation: candidate.Conversation,
		Root: candidate.Thread,
	})
	if errors.Is(err, ticket.ErrNotFound) {
		return TicketIntent{}, false, nil
	}
	if err != nil {
		return TicketIntent{}, false, fmt.Errorf("channel: find ticket reply: %w", err)
	}
	return TicketIntent{Key: held.Key}, true, nil
}

func (r *TicketRoutes) root(
	ctx context.Context, candidate TicketCandidate,
) (TicketIntent, bool, error) {
	if r.settings == nil || r.tickets == nil {
		return TicketIntent{}, false, nil
	}
	name := ConversationKey(candidate.Connection, candidate.Conversation)
	rows, err := r.settings.Named(ctx, KindConversation, name)
	if err != nil {
		return TicketIntent{}, false, fmt.Errorf("channel: read ticket route: %w", err)
	}
	if len(rows) != 1 {
		if len(rows) > 1 {
			return TicketIntent{}, false, ErrAmbiguousConversation
		}
		return TicketIntent{}, false, nil
	}
	set := rows[0]
	var value conversationValue
	if err := json.Unmarshal(set.Value, &value); err != nil ||
		value.Channel != candidate.Connection || value.Mode != ConversationTicket || value.Ticket == nil {
		return TicketIntent{}, false, nil
	}
	compiled, err := r.matcher(name, set.UpdatedAt, *value.Ticket)
	if err != nil {
		return TicketIntent{}, false, fmt.Errorf("channel: compile stored ticket route: %w", err)
	}
	if !matchesTicket(compiled.patterns, candidate.Text) {
		return TicketIntent{}, false, nil
	}
	key, err := ticket.Key(candidate.Connection, candidate.Conversation, candidate.Message)
	if err != nil {
		return TicketIntent{}, false, err
	}
	return TicketIntent{
		Key: key, Root: true, Scope: set.Scope,
		Agent: value.Agent, RunAs: value.RunAs, AddressedBy: compiled.rule.AddressFrom,
	}, true, nil
}

func (r *TicketRoutes) matcher(
	key string, updatedAt time.Time, rule TicketRule,
) (compiledTicketRule, error) {
	fingerprint := sha256.Sum256([]byte(rule.OpenFrom + "\x00" + rule.AddressFrom +
		"\x00" + strings.Join(rule.Patterns, "\x00")))
	r.mu.RLock()
	held, ok := r.compiled[key]
	r.mu.RUnlock()
	if ok && held.updatedAt.Equal(updatedAt) && held.fingerprint == fingerprint {
		return held, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if held, ok = r.compiled[key]; ok && held.updatedAt.Equal(updatedAt) &&
		held.fingerprint == fingerprint {
		return held, nil
	}
	patterns, err := CompileTicketPatterns(rule)
	if err != nil {
		return compiledTicketRule{}, err
	}
	compiled := compiledTicketRule{
		updatedAt: updatedAt, fingerprint: fingerprint, patterns: patterns, rule: rule,
	}
	// Configuration names are unbounded over the life of a process. A renamed
	// or deleted conversation must not leave its regexes resident forever.
	// Clearing is deliberately coarse: the next roots recompile at most eight
	// RE2 patterns each, while the cache remains strictly bounded.
	if len(r.compiled) >= maxCompiledTicketRules {
		clear(r.compiled)
	}
	r.compiled[key] = compiled
	return compiled, nil
}

func CompileTicketPatterns(rule TicketRule) ([]*regexp.Regexp, error) {
	if rule.OpenFrom != TicketOpenLinkedUsers || !addressSource(rule.AddressFrom) ||
		len(rule.Patterns) == 0 || len(rule.Patterns) > MaxTicketPatterns {
		return nil, errors.New("channel: incomplete ticket admission rule")
	}
	compiled := make([]*regexp.Regexp, 0, len(rule.Patterns))
	for _, pattern := range rule.Patterns {
		if strings.TrimSpace(pattern) == "" || len(pattern) > MaxTicketPatternBytes ||
			!utf8.ValidString(pattern) {
			return nil, errors.New("channel: invalid ticket pattern")
		}
		re, err := regexp.Compile("(?i:" + pattern + ")")
		if err != nil {
			// The pattern is administrative input. Do not make it part of an
			// error that may be copied into a response or a log.
			return nil, errors.New("channel: invalid ticket pattern")
		}
		if re.MatchString("") {
			return nil, errors.New("channel: ticket pattern may not match empty text")
		}
		compiled = append(compiled, re)
	}
	return compiled, nil
}

func matchesTicket(patterns []*regexp.Regexp, text string) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func addressSource(source string) bool {
	prefix, id, found := strings.Cut(strings.TrimSpace(source), ":")
	return found && len(source) <= 512 && utf8.ValidString(source) &&
		(prefix == "bot" || prefix == "app") && id != "" &&
		!strings.ContainsAny(id, " \t\r\n")
}
