package channel

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrTicketAdmission is what a rule nobody can act on is refused with. One
// sentinel and a sentence: the sentinel says whose fault it is, so the console
// answers 400 rather than 500, and the sentence says which field to look at.
var ErrTicketAdmission = errors.New("channel: this ticket rule is incomplete")

// MaxTicketClosingEmoji bounds the emoji that end a ticket. A short list is a
// rule people can hold in their heads; a long one is an accident waiting for
// whoever reacts with the wrong thing.
const MaxTicketClosingEmoji = 4

const (
	TicketOpenLinkedUsers   = "linked_users"
	TicketOpenMarkedThreads = "marked_threads"
	MaxTicketPatterns       = 8
	MaxTicketPatternBytes   = 256
	MaxTicketMatchBytes     = 8 * 1024
	maxCompiledTicketRules  = 512
)

/*
TicketRule is the admission policy for one conversation.

Two policies, because two shapes of channel exist. Under `linked_users` the
request is the root and a person wrote it. Under `marked_threads` a form bot
posts the request and the team that owns it is named later in the thread, so
the mark admits the thread and RootFrom says whose root may be admitted at
all. Both name one exact bot or app as the addressing source.
*/
type TicketRule struct {
	OpenFrom    string `json:"openFrom"`
	AddressFrom string `json:"addressFrom"`
	// RootFrom is the only author whose root a mark may admit. It belongs to
	// `marked_threads` alone: the mark says which thread is a ticket, and this
	// says which threads could ever have been one.
	RootFrom string `json:"rootFrom,omitempty"`
	// ReviewIn is the conversation this room's tickets are worked in: the
	// draft is reviewed and corrected there, and only what is approved reaches
	// the support thread. Empty means the support thread is the only room.
	ReviewIn string `json:"reviewIn,omitempty"`
	// ClosesOn are the emoji that end a ticket when somebody who may decide
	// puts one on the request. Empty means nothing closes a ticket by
	// reaction, which is how every installation behaved before this existed.
	ClosesOn []string `json:"closesOn,omitempty"`
	Patterns []string `json:"patterns"`
}

func (r *TicketRoutes) matcher(
	key string, updatedAt time.Time, rule TicketRule,
) (compiledTicketRule, error) {
	fingerprint := sha256.Sum256([]byte(rule.OpenFrom + "\x00" + rule.AddressFrom +
		"\x00" + rule.RootFrom + "\x00" + rule.ReviewIn + "\x00" +
		strings.Join(rule.ClosesOn, "\x00") + "\x00" +
		strings.Join(rule.Patterns, "\x00")))
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
	if err := admissible(rule); err != nil {
		return nil, err
	}
	compiled := make([]*regexp.Regexp, 0, len(rule.Patterns))
	for _, pattern := range rule.Patterns {
		if strings.TrimSpace(pattern) == "" || len(pattern) > MaxTicketPatternBytes ||
			!utf8.ValidString(pattern) {
			return nil, fmt.Errorf("%w: a pattern is empty or longer than %d bytes",
				ErrTicketAdmission, MaxTicketPatternBytes)
		}
		re, err := regexp.Compile("(?i:" + pattern + ")")
		if err != nil {
			// The pattern is administrative input. Do not make it part of an
			// error that may be copied into a response or a log.
			return nil, fmt.Errorf("%w: a pattern is not valid RE2", ErrTicketAdmission)
		}
		if re.MatchString("") {
			return nil, fmt.Errorf("%w: a pattern matches empty text, so it would admit everything",
				ErrTicketAdmission)
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

// reviewRoom answers whether the room a rule names could be one. It is a
// conversation id on the same connection, and nothing else is read from it
// here: whether it is the room's own conversation is a question only the
// screen that knows both can answer.
func reviewRoom(conversation string) bool {
	return conversation == strings.TrimSpace(conversation) && len(conversation) <= 512 &&
		utf8.ValidString(conversation) && !strings.ContainsAny(conversation, " \t\r\n")
}

/*
admissible answers whether the rule describes something this platform does.

Field by field, and each with its own sentence: one message for five causes
sends somebody to re-read a form where four of the fields are already right.
A root source under a policy that never reads one is refused rather than
ignored, because stored configuration that describes something the platform
does not do is read back later as a promise it never made.

What is wrong is named; what was typed is not. These values reach logs and
responses, and a rule is administrative input like any other.
*/
func admissible(rule TicketRule) error {
	switch {
	case rule.OpenFrom != TicketOpenLinkedUsers && rule.OpenFrom != TicketOpenMarkedThreads:
		return fmt.Errorf("%w: it declares a way of opening tickets this version does not know",
			ErrTicketAdmission)
	case !addressSource(rule.AddressFrom):
		return fmt.Errorf("%w: the addressing source must be one exact bot:B… or app:A…",
			ErrTicketAdmission)
	case rule.OpenFrom == TicketOpenLinkedUsers && rule.RootFrom != "":
		return fmt.Errorf("%w: tickets opened from a person's root message trust no root source",
			ErrTicketAdmission)
	case rule.OpenFrom == TicketOpenMarkedThreads && !addressSource(rule.RootFrom):
		return fmt.Errorf("%w: a thread admitted by a mark needs the root source it trusts, as bot:B… or app:A…",
			ErrTicketAdmission)
	case !reviewRoom(rule.ReviewIn):
		return fmt.Errorf("%w: the review room must be one conversation id", ErrTicketAdmission)
	case !closingEmoji(rule.ClosesOn):
		return fmt.Errorf("%w: closing emoji are up to %d plain names, without colons",
			ErrTicketAdmission, MaxTicketClosingEmoji)
	case len(rule.Patterns) == 0 || len(rule.Patterns) > MaxTicketPatterns:
		return fmt.Errorf("%w: between one and %d patterns are needed",
			ErrTicketAdmission, MaxTicketPatterns)
	}
	return nil
}

// sameSourceKey compares a source as the platform writes one. Both sides are
// already typed keys — `bot:B…` or `app:A…` — so this is equality, spelled out
// because a configured id and a vendor id may differ in case alone.
func sameSourceKey(key, configured string) bool {
	key, configured = strings.TrimSpace(key), strings.TrimSpace(configured)
	return key != "" && configured != "" && strings.EqualFold(key, configured)
}

/*
closingEmoji answers whether these names could be emoji.

Names as Slack sends them: no colons, no spaces, nothing a person would have to
guess the spelling of. Empty is the ordinary case and means no reaction closes
anything.
*/
func closingEmoji(names []string) bool {
	if len(names) > MaxTicketClosingEmoji {
		return false
	}
	for _, name := range names {
		if name == "" || name != strings.TrimSpace(name) || len(name) > 64 ||
			!utf8.ValidString(name) || strings.ContainsAny(name, " :\t\r\n") {
			return false
		}
	}
	return true
}

// ClosesTicket answers whether this emoji is one the rule ends a ticket on.
func (r TicketRule) ClosesTicket(emoji string) bool {
	return slices.Contains(r.ClosesOn, strings.TrimSpace(emoji))
}

func addressSource(source string) bool {
	prefix, id, found := strings.Cut(strings.TrimSpace(source), ":")
	return found && len(source) <= 512 && utf8.ValidString(source) &&
		(prefix == "bot" || prefix == "app") && id != "" &&
		!strings.ContainsAny(id, " \t\r\n")
}
