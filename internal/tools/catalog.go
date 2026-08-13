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

	// Effect starts at read and only the Curator changes it (PRD DE-13). A
	// tool that silently arrived as "write" because its own server said so
	// would put the classification back in the hands of a third party.
	Effect domain.Effect
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

	content engine.ContentStore
	timeout time.Duration
}

func NewCatalog(content engine.ContentStore) *Catalog {
	return &Catalog{
		sessions: make(map[string]Session),
		entries:  make(map[domain.ToolID]Entry),
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
