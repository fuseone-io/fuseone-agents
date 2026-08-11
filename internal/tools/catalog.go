// Package tools connects registered MCP servers to the agent loop.
//
// It is the only place the platform reaches outside itself, and it enforces
// the rule that makes open authoring safe: what a tool does to the world is
// decided centrally by the Curator, never by the agent's author and never by
// the server that supplies the tool (PRD DE-12).
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// AddServer registers a connected MCP server and imports its tools.
//
// Every imported tool arrives classified as read-only and untrusted. Making a
// tool able to write is a deliberate act by the Curator afterwards — a server
// cannot grant itself write access by describing a tool as one (PRD DE-13).
func (c *Catalog) AddServer(ctx context.Context, name string, session Session) error {
	if name == "" {
		return fmt.Errorf("tools: server needs a name")
	}

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("tools: list tools from %s: %w", name, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.sessions[name] = session
	for _, t := range listed.Tools {
		id := domain.ToolID(name + "." + t.Name)
		c.entries[id] = Entry{
			ID:          id,
			Server:      name,
			RemoteName:  t.Name,
			Description: t.Description,
			Schema:      schemaProperties(t.InputSchema),
			Effect:      domain.EffectRead,
			Untrusted:   true,
		}
	}
	return nil
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
		})
	}
	slices.SortFunc(out, func(a, b domain.ToolEntry) int {
		return strings.Compare(string(a.ID), string(b.ID))
	})
	return out
}

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
	for _, r := range rulings {
		entry, ok := c.entries[r.Tool]
		if !ok || !r.Effect.Valid() {
			continue
		}
		entry.Effect = r.Effect
		entry.Untrusted = r.Untrusted
		c.entries[r.Tool] = entry
		applied++
	}
	return applied, nil
}

// Classify records what a tool does to the world.
//
// This is the Curator's act and the single point where write access enters the
// system. It is deliberately separate from registration so that importing a
// server can never widen what agents may do.
func (c *Catalog) Classify(id domain.ToolID, effect domain.Effect, untrusted bool) error {
	if !effect.Valid() {
		return fmt.Errorf("tools: %q is not a valid effect classification", effect)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownTool, id)
	}
	entry.Effect = effect
	entry.Untrusted = untrusted
	c.entries[id] = entry
	return nil
}

// Effect answers the Gate's first question about a tool.
//
// An unknown tool returns false, and the Gate blocks: a tool nobody classified
// never executes (PRD DE-12).
func (c *Catalog) Effect(id domain.ToolID) (domain.Effect, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[id]
	if !ok {
		return domain.EffectUnknown, false
	}
	return entry.Effect, true
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

// Invoke calls a tool and returns what came back.
//
// The result never lands in the ledger: it goes to the content store and the
// ledger records a reference. Tool output routinely carries personal data, and
// inlining it would make retention impossible to honour (PRD AU-04).
func (c *Catalog) Invoke(ctx context.Context, call engine.Call) (engine.ToolResult, error) {
	c.mu.RLock()
	entry, known := c.entries[call.Tool]
	session, connected := c.sessions[entry.Server]
	timeout := c.timeout
	c.mu.RUnlock()

	if !known {
		return engine.ToolResult{}, fmt.Errorf("%w: %s", ErrUnknownTool, call.Tool)
	}
	if !connected {
		return engine.ToolResult{}, fmt.Errorf("%w: %s", ErrUnknownServer, entry.Server)
	}

	var args any
	if len(call.Args) > 0 {
		if err := json.Unmarshal(call.Args, &args); err != nil {
			return engine.ToolResult{}, fmt.Errorf("tools: arguments for %s are not valid JSON: %w", call.Tool, err)
		}
	}

	// A tool that never returns would hold a worker's slot until the lease
	// expires; bound it here rather than relying on the server to behave.
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := session.CallTool(callCtx, &mcp.CallToolParams{
		Name:      entry.RemoteName,
		Arguments: args,
	})
	if err != nil {
		return engine.ToolResult{}, fmt.Errorf("tools: call %s: %w", call.Tool, err)
	}

	out := engine.ToolResult{Failed: res.IsError}
	if res.IsError {
		out.ErrorCode = "tool_error"
	}
	// Output from a source the Curator has not vouched for is tainted from the
	// moment it enters. Everything derived from it inherits the label, which
	// is what stops an attacker-authored document steering a later action.
	if entry.Untrusted {
		out.Labels = domain.NewLabels(domain.LabelUntrusted)
	}

	text := flatten(res)
	if c.content != nil && text != "" {
		ref, err := c.content.Put(ctx, call.RunID, call.Seq, []byte(text))
		if err != nil {
			return engine.ToolResult{}, fmt.Errorf("tools: store result of %s: %w", call.Tool, err)
		}
		out.ResultRef = ref
	}
	return out, nil
}

// Close shuts every session down.
func (c *Catalog) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var errs []error
	for name, s := range c.sessions {
		if err := s.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", name, err))
		}
	}
	clear(c.sessions)
	return errors.Join(errs...)
}

// flatten renders a tool result as text for the model.
func flatten(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, content := range res.Content {
		if t, ok := content.(*mcp.TextContent); ok {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(t.Text)
		}
	}
	if b.Len() == 0 && res.StructuredContent != nil {
		if raw, err := json.Marshal(res.StructuredContent); err == nil {
			b.Write(raw)
		}
	}
	return b.String()
}

// schemaProperties pulls the properties map out of a tool's JSON Schema. The
// model needs the field descriptions; the surrounding envelope it does not.
func schemaProperties(schema any) map[string]any {
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var decoded struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	return decoded.Properties
}
