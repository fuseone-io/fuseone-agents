// Package tools connects registered MCP servers to the agent loop.
//
// It is the only place the platform reaches outside itself, and it enforces
// the rule that makes open authoring safe: what a tool does to the world is
// decided centrally by the Curator, never by the agent's author and never by
// the server that supplies the tool (PRD DE-12).
package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
The tool servers this installation is connected to.

Registering is the Curator's act and it grants nothing: a tool arrives
unclassified whatever its server claims about it, and does nothing at all until
somebody rules on it (PRD DE-12, DE-13).
*/
// AddServer registers a connected MCP server and imports its tools.
//
// Every imported tool arrives unclassified and untrusted, and the Gate refuses
// an unclassified tool at the contract check. It used to arrive read-only,
// which reads as a restriction and behaves as a permission — read is allowed,
// so importing a server created one permitted tool per name it offered.
//
// A server still cannot grant itself write access by describing a tool as one.
// That was always the argument, and unclassified is what it argues for:
// refusing the server's claim without acting on it in either direction.
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
			Digest:      digestOf(name, t.Name, t.Description, schemaProperties(t.InputSchema)),
			Effect:      domain.EffectUnknown,
			Untrusted:   true,
			Suggested:   c.suggestion(name, t.Name),
		}
	}
	return nil
}

// RemoveServer disconnects a server and drops its tools.
//
// Both halves matter. A tool left behind by a server nobody is connected to
// would be offered to a planner, ruled on by the Gate, and then fail at the
// call — a refusal the trail explains badly. And a session left open is a
// process this installation started that keeps running until somebody stops
// it.
//
// Removing one that was never connected is not an error: the reconciler asks
// from a desired state rather than from knowledge of what is connected.
func (c *Catalog) RemoveServer(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	session, connected := c.sessions[name]
	if !connected {
		return nil
	}
	delete(c.sessions, name)
	for id, entry := range c.entries {
		if entry.Server == name {
			delete(c.entries, id)
		}
	}
	if err := session.Close(); err != nil {
		return fmt.Errorf("tools: close %s: %w", name, err)
	}
	return nil
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

// suggestion is what the platform already believes about this tool, if
// anything.
//
// Matched by the name the server answered with, so an entry that has aged
// degrades into silence rather than into a wrong answer — the worst a stale
// suggestion can do is leave the Curator where they would have been without
// one. Called with the lock held.
func (c *Catalog) suggestion(server, remoteName string) *Suggestion {
	if c.known == nil {
		return nil
	}
	found, ok := c.known.Suggest(server, remoteName)
	if !ok {
		return nil
	}

	effect, err := domain.ParseEffect(found.Effect)
	if err != nil {
		// A shipped effect the domain does not know is a typo in our own data,
		// and the safe reading of a typo is no opinion at all.
		return nil
	}
	return &Suggestion{
		Effect:        effect,
		Untrusted:     found.Untrusted,
		CompensatedBy: domain.ToolID(found.CompensatedBy),
		Why:           found.Why,
	}
}

/*
digestOf names one tool definition, stably and across processes.

What goes in is what a Curator reads before deciding: which server offers it,
what it calls itself, the sentence it describes itself with, and the arguments
it accepts. What stays out is anything that varies without the tool varying —
the order a server happened to list its tools in, a session id, a timestamp.

Canonical because `encoding/json` sorts map keys: two processes discovering the
same tool must compute the same digest, or every reconnect would look like a
change and every ruling would go stale on a restart.
*/
func digestOf(server, name, description string, schema map[string]any) string {
	shape := struct {
		Server      string         `json:"server"`
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Schema      map[string]any `json:"schema"`
	}{server, name, description, schema}

	canonical, err := json.Marshal(shape)
	if err != nil {
		// A schema that will not encode is one nobody can judge either. Empty
		// reads as "no digest recorded", which applies the ruling — so it is
		// refused instead, by never matching a recorded one.
		return "unencodable"
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}
