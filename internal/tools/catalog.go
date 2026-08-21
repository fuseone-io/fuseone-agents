// Package tools connects registered MCP servers to the agent loop.
//
// It is the only place the platform reaches outside itself, and it enforces
// the rule that makes open authoring safe: what a tool does to the world is
// decided centrally by the Curator, never by the agent's author and never by
// the server that supplies the tool (PRD DE-12).
package tools

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/known"
)

var (
	ErrUnknownTool   = errors.New("tools: no such tool in the catalogue")
	ErrUnknownServer = errors.New("tools: no such server")
)

// Session is the part of an MCP client session the catalogue uses. Declared
// here so a test can stand in for a real server without a subprocess.
type Session interface {
	ListTools(ctx context.Context, params *mcp.ListToolsParams) (*mcp.ListToolsResult, error)
	CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error)
	Close() error
}

// Entry is one tool as the platform sees it.
type Entry struct {
	ID     domain.ToolID
	Server string
	// RemoteName is what the server calls it. The platform namespaces tools by
	// server, because two servers naming a tool "search" must not collide into
	// one capability.
	RemoteName  string
	Description string
	Schema      map[string]any
	/*
		Digest is this definition, as offered right now.

		Computed at discovery from what the server said — the name it uses, the
		sentence it describes itself with, the arguments it accepts — because
		those are what a Curator reads to decide. It is what a ruling is
		matched against, so that a ruling applies to the tool it was made about
		and not to whatever now answers to the same name.
	*/
	Digest string
	/*
		Stale marks a tool whose ruling was overtaken by a new definition.

		Refused either way — the effect stays unclassified and the Gate already
		knows what to do with that. It exists because the two refusals are
		different work: a tool nobody ruled on is a decision to make, and a
		tool that changed under its ruling is a decision to check. Shown as the
		same thing, the second reads as somebody having forgotten.
	*/
	Stale bool

	/*
		Effect starts unclassified and only the Curator sets it (PRD DE-13).

		A tool that silently arrived as "write" because its own server said so
		would put the classification back in the hands of a third party. That
		argument is right and it does not argue for read: read is *allowed*, so
		importing a server used to create one permitted tool per name it offered —
		`delete_repository` among them — until somebody ruled on each. The label
		read like a restriction and behaved as a permission.

		Unclassified refuses the server's claim without acting on it either way,
		and the Gate already knows what to do with it.
	*/
	Effect domain.Effect
	/*
		Suggested is what the platform ships about a known server, and it is not a
		classification.

		Applied on import it would put the decision back in a table shipped in a
		binary — the same mistake as trusting the server, one step further away and
		harder to see. What it saves is the Curator inventing forty rulings from a
		list of bare names, which is how a safe default becomes one people work
		around. Nil for a server nobody catalogued, and for a tool an entry never
		heard of.
	*/
	/*
		OnSurface is whether this installation brought the tool in.

		Not a permission and not a policy. A tool outside the surface is not
		"allowed with conditions" — it is not here: no model is told about it,
		no call reaches it, and the Gate is never asked, because the Gate
		decides between an agent and a capability this installation has.

		It exists because a server with two hundred tools is otherwise two
		hundred decisions, most of them about tools nobody wants. Choosing the
		surface is choosing what goes to the Curator at all.
	*/
	OnSurface bool
	Suggested *Suggestion
	// Untrusted marks a source whose output may be attacker-authored. It is
	// the default for anything registered from outside, and it is what makes
	// taint propagate into the run (PRD DE-14, SE-05).
	Untrusted bool
	// CompensatedBy is what takes an act by this tool back. The Curator rules
	// on it alongside the effect, because what a tool does to the world and
	// how to undo it are one judgement (PRD SE-08).
	CompensatedBy domain.ToolID
}

// Catalog is the registered tool surface of an installation.
type Catalog struct {
	mu       sync.RWMutex
	sessions map[string]Session
	entries  map[domain.ToolID]Entry
	limiters map[string]*serverLimiter

	content engine.ContentStore
	timeout time.Duration
	// known is what the platform ships about servers other people publish.
	// Optional: without it every tool imports with no suggestion, which is
	// what happened before there was a catalogue.
	known Suggester
}

// Suggester is what the platform already knows about a server's tools,
// declared here by the consumer.
type Suggester interface {
	Suggest(server, remoteName string) (known.Suggestion, bool)
}

// Suggestion is a shipped opinion about one tool, resolved into domain types.
type Suggestion struct {
	Effect        domain.Effect
	Untrusted     *bool
	CompensatedBy domain.ToolID
	Why           string
}

// Knowing wires the shipped catalogue.
func (c *Catalog) Knowing(known Suggester) *Catalog {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.known = known
	return c
}

// Lookup returns one entry, for the console and for the Curator's screen.
func (c *Catalog) Lookup(id domain.ToolID) (Entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[id]
	return e, ok
}

func NewCatalog(content engine.ContentStore) *Catalog {
	return &Catalog{
		sessions: make(map[string]Session),
		entries:  make(map[domain.ToolID]Entry),
		limiters: make(map[string]*serverLimiter),
		content:  content,
		timeout:  60 * time.Second,
	}
}

var (
	_ engine.Catalog   = (*Catalog)(nil)
	_ engine.Tools     = (*Catalog)(nil)
	_ modelToolSchemas = (*Catalog)(nil)
)

// modelToolSchemas mirrors model.ToolSchemas. It is restated rather than
// imported so this package does not depend on the model package; Go interfaces
// are structural, so the compile-time assertion above still holds.
type modelToolSchemas interface {
	Schema(domain.ToolID) (name, description string, input map[string]any, ok bool)
}

// Entries renders the catalogue for the administration area: what the platform
// knows, where each tool came from, and what somebody decided it does.
func (c *Catalog) Entries() []domain.ToolEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]domain.ToolEntry, 0, len(c.entries))
	for _, e := range c.entries {
		out = append(out, domain.ToolEntry{
			ID: e.ID, Server: e.Server, Description: e.Description,
			Effect: e.Effect, Untrusted: e.Untrusted,
			CompensatedBy: e.CompensatedBy,
			Suggested:     suggestionOf(e.Suggested),
			Digest:        e.Digest, Stale: e.Stale, OnSurface: e.OnSurface,
		})
	}
	slices.SortFunc(out, func(a, b domain.ToolEntry) int {
		return strings.Compare(string(a.ID), string(b.ID))
	})
	return out
}

// CountFrom reports how many tools one server offered.
//
// A reachable server offering nothing is a real and confusing state — usually
// a server that started but has not finished registering — so the count is
// reported separately from whether it answered.
func (c *Catalog) CountFrom(server string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	count := 0
	for _, e := range c.entries {
		if e.Server == server {
			count++
		}
	}
	return count
}

// Schema describes a tool to the model.
func (c *Catalog) Schema(id domain.ToolID) (string, string, map[string]any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[id]
	if !ok {
		return "", "", nil, false
	}
	if !entry.OnSurface {
		// Never described. A schema for a tool that cannot be called is an
		// invitation to call it, and the model will take it.
		return "", "", nil, false
	}
	return string(entry.ID), entry.Description, entry.Schema, true
}

// List returns the catalogue, for the Curator's console.
func (c *Catalog) List() []Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]Entry, 0, len(c.entries))
	for _, e := range c.entries {
		out = append(out, e)
	}
	return out
}

// suggestionOf carries a shipped opinion into the administration read model.
//
// It travels or the catalogue is data nobody reads: the promise is that the
// Curator sees the reasoning before confirming, and a suggestion the screen
// never receives keeps none of it.
func suggestionOf(s *Suggestion) *domain.ToolSuggestion {
	if s == nil {
		return nil
	}
	untrusted := true
	if s.Untrusted != nil {
		untrusted = *s.Untrusted
	}
	return &domain.ToolSuggestion{
		Effect: s.Effect, Untrusted: untrusted,
		CompensatedBy: s.CompensatedBy, Why: s.Why,
	}
}
