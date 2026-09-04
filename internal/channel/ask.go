package channel

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

/*
Reading an ask out of a message.

The mention names the agent, which is the whole reason a channel trigger needs
no configuration: what governs whether an agent may be started here is the
intersection of two facts that already exist — the conversation maps to a
scope, and the agent lives in it (NT-005 §9).

**Nothing here is inferred, and the name is required wherever nobody decided
otherwise.** With one startable agent in a scope, picking it looks like the
only possible reading and is still the platform deciding: the day a second
agent is added, the same sentence means something else and nobody changed it.
Every other place here refuses to guess — the exception is not read out of
prose, the scope is not read out of the text — and an agent that runs because
it happened to be the only one is the same shape of guess.

A conversation bound to an agent is the opposite of that. An administrator
wrote it down against that conversation, in the same configuration that decides
the scope, and reading it is no more a guess than reading the scope is. So a
mention there needs no name, and the whole sentence is the ask.

Where nobody decided, the refusal is what teaches. It names what is startable
in this conversation, so the first attempt is the last one that fails.
*/

// ErrNoAgentNamed means the message did not say which agent it was for.
var ErrNoAgentNamed = errors.New("channel: the message names no agent")

// ErrNotStartable means the named agent cannot be started from here: it does
// not exist in this conversation's scope, or it never declared willingness.
//
// One error for both, and deliberately: an answer that distinguished them would
// tell somebody which agents exist in a scope they cannot reach, from a channel
// anybody in it can type into.
var ErrNotStartable = errors.New("channel: no agent by that name can be started here")

// mention is how a channel writes a mention of the bot. Stripped before the
// name is read, because what somebody typed is the ask and the mention is the
// envelope it arrived in.
var mention = regexp.MustCompile(`<@[A-Z0-9]+>`)

// Startable is one agent an ask in this conversation may start.
type Startable struct {
	ID   domain.AgentID
	Name string
}

// Ask is a message read as a request: who it is for, and what was said.
type Ask struct {
	Agent domain.AgentID
	Text  string
}

/*
Read resolves a message into an ask, or refuses.

Matched against the id and against the name, because an operator knows an agent
by its id and its author named it in their own language. By prefix rather than
by first word: a name is usually several — "Triagem de chamados" — and matching
one word would leave the name half-supported, working for the agents somebody
happened to name in a single word and silently never for the rest.

Longest first, so an agent whose name begins with another's resolves to the one
actually written.

The bound agent is what the conversation was configured to start, and it is
reached only after every name has failed to match. A default that outranked the
name somebody typed would make the name a decoration.
*/
func Read(text string, startable []Startable, bound domain.AgentID) (Ask, error) {
	said := strings.TrimSpace(mention.ReplaceAllString(text, " "))

	for _, form := range addressings(startable) {
		if rest, ok := after(said, form.text); ok {
			return Ask{Agent: form.agent, Text: rest}, nil
		}
	}
	if bound != "" {
		// Nothing was consumed as a name, because nothing in it was one. A bare
		// mention arrives here too and resolves with no words; whether that is
		// worth a run depends on what came before it in the conversation, which
		// this cannot see and the consumer can.
		return Ask{Agent: bound, Text: said}, nil
	}

	if said == "" {
		return Ask{}, fmt.Errorf("%w. %s", ErrNoAgentNamed, listing(startable))
	}
	first, _, _ := strings.Cut(said, " ")
	return Ask{}, fmt.Errorf("%w: %q. %s", ErrNotStartable, first, listing(startable))
}

// after reports the rest of a message addressed to this name.
//
// The separator matters: `triagemzinho` is not `triagem`, and reading it as one
// would send an ask to an agent nobody addressed. Punctuation somebody would
// naturally type after a name — a colon, a comma — is consumed with it.
func after(said, name string) (rest string, ok bool) {
	if name == "" || len(said) < len(name) {
		return "", false
	}
	if !strings.EqualFold(said[:len(name)], name) {
		return "", false
	}
	rest = said[len(name):]
	if rest == "" {
		return "", true
	}
	if !strings.ContainsRune(" \t\n:,", rune(rest[0])) {
		return "", false
	}
	return strings.TrimLeft(strings.TrimSpace(rest), ":, \t\n"), true
}

// addressing is one way of writing an agent's name, and whose it is.
type addressing struct {
	text  string
	agent domain.AgentID
}

/*
addressings is every way an agent here can be addressed, longest first.

Longest across all of them and not per agent: an agent's own id is usually a
prefix of its own name — `triagem` and "Triagem de chamados" — so trying a
whole agent's forms before the next agent's would match the id and leave the
rest of its name sitting in the ask. Sorting the forms themselves is what makes
the longer reading win wherever it comes from, which is also what resolves one
agent named inside another's name.
*/
func addressings(startable []Startable) []addressing {
	out := make([]addressing, 0, len(startable)*2)
	for _, one := range startable {
		out = append(out, addressing{text: string(one.ID), agent: one.ID})
		if one.Name != "" {
			out = append(out, addressing{text: one.Name, agent: one.ID})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i].text) > len(out[j].text)
	})
	return out
}

// listing is what can be started here, for the refusal.
//
// The refusal is the only teaching surface a channel has: nobody reads
// documentation before typing in a chat, and a message that says "no" without
// saying what would have worked is one somebody gives up after. Ids rather
// than names, because the id is the form that always works.
func listing(startable []Startable) string {
	if len(startable) == 0 {
		return "No agent in this conversation's area is startable by a message."
	}
	names := make([]string, 0, len(startable))
	for _, one := range startable {
		names = append(names, string(one.ID))
	}
	return "Startable here: " + strings.Join(names, ", ") + "."
}
