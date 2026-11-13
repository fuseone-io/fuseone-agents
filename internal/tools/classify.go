// Package tools connects registered MCP servers to the agent loop.
//
// It is the only place the platform reaches outside itself, and it enforces
// the rule that makes open authoring safe: what a tool does to the world is
// decided centrally by the Curator, never by the agent's author and never by
// the server that supplies the tool (PRD DE-12).
package tools

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
What the Curator decided each tool does.

Held beside the catalogue rather than inside the entry it describes, because
the two have different lifetimes: a server comes and goes with a connection,
and a ruling outlives every process that ever read it.
*/
// Classifier is where rulings come from, declared here by the consumer so the
// administration that records them and the catalogue that enforces them never
// import each other.
type Classifier interface {
	List(ctx context.Context, scope domain.Scope) ([]domain.ToolClassification, error)
}

// Sync applies every recorded ruling to the catalogue.
//
// It is how a promotion made in the administration area reaches the process
// that enforces it. A ruling for a tool this catalogue does not carry is
// ignored rather than an error: servers come and go, and a stale ruling for an
// absent tool is not a reason to refuse every current one.
func (c *Catalog) Sync(ctx context.Context, from Classifier, scope domain.Scope) (int, error) {
	rulings, err := from.List(ctx, scope)
	if err != nil {
		return 0, fmt.Errorf("tools: read rulings: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	applied := 0
	decided := map[domain.ToolID]bool{}
	for _, r := range c.rulingsInForce(rulings) {
		entry, ok := c.entries[r.Tool]
		if !ok || !r.Effect.Valid() || r.Dedupe.Validate() != nil {
			continue
		}
		decided[r.Tool] = true
		/*
			The ruling has to be about this definition.

			A server may keep a name and change everything under it, and the
			ruling that arrives here was made by somebody reading the old
			description. Applied anyway, an effect reaches production that
			nobody judged — quietly, on a reconnect, with no screen mentioning
			it.

			An empty digest is a ruling recorded before this was kept and it
			still applies, or every installation would upgrade into an outage.
		*/
		if r.Digest != "" && r.Digest != entry.Digest {
			// Back to how discovery leaves a tool, not merely flagged: what an
			// earlier ruling set would otherwise keep deciding. With two
			// rulings per tool (NT-019) a stale ceiling can arrive after the
			// owner's took effect, and keeping the owner's read is the
			// fall-back the ceiling rules out.
			entry.Stale = true
			entry.Effect = domain.EffectUnknown
			entry.Untrusted = true
			entry.CompensatedBy = ""
			entry.Dedupe = domain.ToolDedupe{}
			c.entries[r.Tool] = entry
			continue
		}
		entry.Stale = false
		entry.Effect = r.Effect
		entry.Untrusted = r.Untrusted
		entry.CompensatedBy = r.CompensatedBy
		entry.Dedupe = r.Dedupe.Clone()
		c.entries[r.Tool] = entry
		applied++
	}
	c.forgetUndecidedLocked(decided)
	return applied, nil
}

/*
forgetUndecidedLocked puts every tool no ruling decides back to how discovery
leaves it (NT-019).

Rulings used to be only ever replaced. The platform's ceiling can now be
withdrawn, and a tool with no owner's ruling under it then has none: kept as
it was, the withdrawn ceiling would go on deciding it in every running worker
until the worker restarted. The same holds for an owner's ruling whose server
moved to another scope, which is nobody's word any more.
*/
func (c *Catalog) forgetUndecidedLocked(decided map[domain.ToolID]bool) {
	for id, entry := range c.entries {
		if decided[id] {
			continue
		}
		entry.Stale = false
		entry.Effect = domain.EffectUnknown
		entry.Untrusted = true
		entry.CompensatedBy = ""
		entry.Dedupe = domain.ToolDedupe{}
		c.entries[id] = entry
	}
}

// Classify records what a tool does to the world.
//
// This is the Curator's act and the single point where write access enters the
// system. It is deliberately separate from registration so that importing a
// server can never widen what agents may do.
func (c *Catalog) Classify(ruling domain.ToolClassification) error {
	if !ruling.Effect.Valid() {
		return fmt.Errorf("tools: %q is not a valid effect classification", ruling.Effect)
	}
	if err := ruling.Dedupe.Validate(); err != nil {
		return fmt.Errorf("tools: invalid dedupe for %s: %w", ruling.Tool, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[ruling.Tool]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownTool, ruling.Tool)
	}
	entry.Effect = ruling.Effect
	entry.Untrusted = ruling.Untrusted
	entry.CompensatedBy = ruling.CompensatedBy
	entry.Dedupe = ruling.Dedupe.Clone()
	c.entries[ruling.Tool] = entry
	return nil
}

// CompensatedBy answers what undoes a tool, for the abandonment path.
func (c *Catalog) CompensatedBy(id domain.ToolID) (domain.ToolID, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[id]
	if !ok || entry.CompensatedBy == "" {
		return "", false
	}
	return entry.CompensatedBy, true
}

// Effect answers the Gate's first question about a tool.
//
// An unknown tool returns false, and the Gate blocks: a tool nobody classified
// never executes (PRD DE-12). So does one outside this installation's surface,
// which is not a refusal but an absence — the Gate is never asked about a
// capability the platform does not have.
func (c *Catalog) Effect(id domain.ToolID) (domain.Effect, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[id]
	if !ok || !entry.OnSurface {
		// Off the surface answers the same as absent, and that is the honest
		// shape: the Gate decides between an agent and a capability this
		// installation has, and a tool nobody brought in is not one.
		return domain.EffectUnknown, false
	}
	return entry.Effect, true
}

// Dedupe answers whether a ruled tool has a Curator-declared semantic key for
// cross-run dedupe. Unknown, off-surface and undeclared tools all return false:
// the safe default is ordinary execution, not a raw-argument hash fallback.
func (c *Catalog) Dedupe(id domain.ToolID) (domain.ToolDedupe, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[id]
	if !ok || !entry.OnSurface || !entry.Dedupe.Enabled() {
		return domain.ToolDedupe{}, false
	}
	return entry.Dedupe.Clone(), true
}
