package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
)

// ErrSessionReplaced is a rediscovery whose answer came from a session the
// catalogue no longer holds: a reconnect happened while it was listing, and the
// new session's own discovery is the one that counts.
var ErrSessionReplaced = errors.New("tools: the session was replaced during rediscovery")

/*
Rediscover asks a connected server again what it offers, on the session it
already has open (#287).

It is not AddServer with the same session. Nothing is closed or reconnected, so
a call in flight finishes on the session it started on, and the limiter, the
timeout and the result cache stay as configured. A tool whose definition is
unchanged keeps the entry it had, ruling included, so a call made between this
and the next Sync is not refused as unknown. A new tool, or one whose
definition changed under its name, arrives as any discovered tool does:
unclassified and untrusted. A tool no longer listed leaves the catalogue.

The listing happens outside the lock, and a failure changes nothing.
*/
func (c *Catalog) Rediscover(ctx context.Context, name string, surface *[]string) error {
	c.mu.RLock()
	session, ok := c.sessions[name]
	c.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownServer, name)
	}

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("tools: rediscover %s: %w", name, err)
	}
	digests, err := digestsOf(name, listed.Tools)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sessions[name] != session {
		return fmt.Errorf("%w: %s", ErrSessionReplaced, name)
	}
	had := make(map[domain.ToolID]Entry)
	for id, entry := range c.entries {
		if entry.Server == name {
			had[id] = entry
			delete(c.entries, id)
		}
	}
	for _, t := range listed.Tools {
		entry := c.newEntry(name, t, surface, digests[t.Name], c.provenance[name])
		if before, kept := had[entry.ID]; kept && before.Digest == entry.Digest {
			before.OnSurface = entry.OnSurface
			entry = before
		}
		c.entries[entry.ID] = entry
	}
	return nil
}

// digestsOf names every definition a server listed, before anything is stored,
// so a definition nobody can hash fails discovery rather than entering the
// catalogue.
func digestsOf(server string, listed []*mcp.Tool) (map[string]string, error) {
	digests := make(map[string]string, len(listed))
	for _, t := range listed {
		digest, err := digestOf(server, t.Name, t.Description, t.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("tools: describe %s.%s: %w", server, t.Name, err)
		}
		digests[t.Name] = digest
	}
	return digests, nil
}

// newEntry is a tool as discovery first records it: unclassified and
// untrusted, whatever its server claims. Called with the lock held.
func (c *Catalog) newEntry(
	server string, t *mcp.Tool, surface *[]string, digest string, provenance domain.ToolProvenance,
) Entry {
	return Entry{
		ID:          domain.ToolID(server + "." + t.Name),
		Server:      server,
		OnSurface:   chosen(surface, t.Name),
		RemoteName:  t.Name,
		Description: t.Description,
		Schema:      schemaProperties(t.InputSchema),
		Digest:      digest,
		Effect:      domain.EffectUnknown,
		Untrusted:   true,
		Suggested:   c.suggestion(server, t.Name),
		Provenance:  provenance,
	}
}
