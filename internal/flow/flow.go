/*
Package flow answers a question about a specification without running it
(PRD SE-07): does data the platform did not author reach a tool that acts on
the world?

The Gate answers the same question at runtime, per call, with the actual taint.
That is the enforcement and it is not going away. This is the earlier, cheaper
answer: an author about to publish should find out that their agent reads a
customer's email and then writes to a ticket, rather than finding out from an
approval queue that fills up on Monday.

It reports rather than refuses. The path it finds is usually the point of the
agent — reading something and acting on it is what these things are for — and
a check that blocked publication would be turned off within a week. What it
buys is that nobody is surprised.
*/
package flow

import (
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// Catalogue is what each tool does and where its results come from, declared
// here by the consumer.
type Catalogue interface {
	Effect(domain.ToolID) (domain.Effect, bool)
	Untrusted(domain.ToolID) bool
}

// Envelope is one step of a specification, as data: the package cannot import
// the one that parses definitions.
type Envelope struct {
	Name    string
	Reaches []domain.ToolID
}

// Path is one way tainted data can reach an act.
type Path struct {
	// From is the tool that brings data in, To is the tool that acts on the
	// world with it.
	From, To domain.ToolID
	// FromStep and ToStep name where each sits. Equal when both are reachable
	// in the same step, which is the common case and the least constrained
	// one: within a step the model chooses the order.
	FromStep, ToStep string
	// Effect is what the acting tool does, which is how bad this is.
	Effect domain.Effect
}

func (p Path) String() string {
	return fmt.Sprintf("%s → %s (%s)", p.From, p.To, p.Effect)
}

// Finding is something worth saying about a specification before it is
// published.
type Finding struct {
	// Paths are every way untrusted data reaches an effectful tool.
	Paths []Path
	// Unclassified are tools the Curator has not ruled on. They read as
	// read-only and untrusted, which is the safe default and also means this
	// check cannot say anything useful about them.
	Unclassified []domain.ToolID
}

/*
Check reads a specification's tools and steps and reports the paths.

Order comes from the steps. Taint from step two reaches steps two, three and
four, and never step one — that is the whole reason envelopes exist. A
specification with no steps is one envelope holding everything, where any read
can precede any write.
*/
func Check(pack []domain.ToolID, steps []Envelope, catalogue Catalogue) Finding {
	if len(steps) == 0 {
		steps = []Envelope{{Reaches: pack}}
	}

	var finding Finding
	seen := map[domain.ToolID]bool{}
	for _, tool := range pack {
		if _, ok := catalogue.Effect(tool); !ok && !seen[tool] {
			seen[tool] = true
			finding.Unclassified = append(finding.Unclassified, tool)
		}
	}

	// Sources carried forward: a step's reads are available to it and to
	// everything after it.
	var sources []Path
	for _, step := range steps {
		reaches := step.Reaches
		if len(reaches) == 0 {
			reaches = pack
		}

		// Within the step, both directions: the model decides the order, so a
		// read and a write in the same envelope are a path whichever way the
		// author listed them.
		for _, tool := range reaches {
			if !catalogue.Untrusted(tool) {
				continue
			}
			sources = append(sources, Path{From: tool, FromStep: step.Name})
		}

		for _, tool := range reaches {
			effect, _ := catalogue.Effect(tool)
			if effect == domain.EffectRead || !effect.Valid() {
				continue
			}
			for _, source := range sources {
				finding.Paths = append(finding.Paths, Path{
					From: source.From, FromStep: source.FromStep,
					To: tool, ToStep: step.Name, Effect: effect,
				})
			}
		}
	}
	return finding
}

// Clean reports whether there is nothing to say.
func (f Finding) Clean() bool {
	return len(f.Paths) == 0 && len(f.Unclassified) == 0
}
