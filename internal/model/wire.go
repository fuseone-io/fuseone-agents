package model

import (
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

/*
The name a tool goes by on the wire.

A tool identifier is `server.tool`, and no provider accepts the dot: both wire
formats bound a function name to `[A-Za-z0-9_-]`. Sent as it is, every agent
holding any tool at all is refused with a 400 before the model reads a word of
the prompt.

So the name on the wire is a rendering, the way a chip in the editor is a
rendering of an identifier, and what the Gate rules on is always the identifier
the catalogue issued. The two directions are one map built from the pack of
this request — never an encoding inverted by guessing, because a guess that is
wrong routes a call to a tool nobody authorised.
*/

// names carries a request's tools in both directions.
type names struct {
	wire map[domain.ToolID]string
	tool map[string]domain.ToolID
}

/*
namesFor assigns a name to everything this request will mention.

The pack of the step being planned, and then everything the transcript already
did. The two are not the same set: a call made under step one is still in the
history when step two is the one being planned, and named from the offered set
alone it would go out with no name at all — which is the same 400 as the dot,
arriving precisely when a run has just succeeded at something.

Naming is not offering. What the model may call is still only what this step
reaches; what it may be reminded of is everything it already did. The pack is
the Gate's business and the transcript must not widen it by the back door.
*/
func namesFor(in engine.PlanInput) names {
	n := names{
		wire: make(map[domain.ToolID]string, len(in.Tools)),
		tool: make(map[string]domain.ToolID, len(in.Tools)),
	}
	n.add(in.Tools...)
	for _, turn := range in.Transcript {
		n.add(turn.Tool)
	}
	return n
}

func (n names) add(ids ...domain.ToolID) {
	for _, id := range ids {
		if _, known := n.wire[id]; known || id == "" {
			continue
		}
		name := n.free(safeName(string(id)))
		n.wire[id] = name
		n.tool[name] = id
	}
}

// free is the first name in this request nothing else answers to.
//
// Two identifiers can render to one name — `crm.lookup` and `crm_lookup` do —
// and a collision left alone would send one tool's arguments to the other. It
// is resolved rather than detected because the pack is not the author's to
// change at the moment a run needs it.
func (n names) free(name string) string {
	if _, taken := n.tool[name]; !taken {
		return name
	}
	for suffix := 2; ; suffix++ {
		candidate := name + "_" + string(rune('0'+suffix%10))
		if _, taken := n.tool[candidate]; !taken {
			return candidate
		}
	}
}

// idOf reads a proposal's name back.
//
// A name nobody offered comes back exactly as the model said it: the trail
// records what was proposed rather than the nearest thing it resembles, and
// the Gate refuses it for not being in the pack — which is the right answer to
// a call the model invented.
func (n names) idOf(name string) domain.ToolID {
	if id, ok := n.tool[name]; ok {
		return id
	}
	return domain.ToolID(name)
}

// safeName replaces everything a provider will not take.
func safeName(id string) string {
	var b strings.Builder
	b.Grow(len(id))
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == '.':
			// Doubled, so `crm.lookup` reads as two parts rather than as one
			// word — the model is being told what it may call, and the name is
			// the only description of it that is always present.
			b.WriteString("__")
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
