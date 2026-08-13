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
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
The tool servers this installation is connected to.

Registering is the Curator’s act and it grants nothing: a tool arrives
read-only whatever its server claims about it, and becomes anything else only
when somebody rules on it (PRD DE-12, DE-13).
*/
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
