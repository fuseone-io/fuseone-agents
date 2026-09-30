package tools

import (
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

/*
A session that has died stays dead.

The SDK closes an MCP connection for good on some failures — the one seen in
the field is a server answering 400 to the "notifications/cancelled" the SDK
sends when a call times out. From then on every call on that session fails with
ErrConnectionClosed, and nothing reconnects it: the reconciler only compares
configuration, and the configuration has not changed. One slow query used to
break a server until somebody restarted the worker.

So the catalogue retires a session the moment it reports its connection closed,
and the reconciler reconnects whatever is configured but no longer connected.
*/

// liveSession gives every registered session a pointer identity, so a failure
// on a stale session can be told apart from the session that replaced it.
// Comparing the sessions themselves could panic: a Session may be a struct
// holding a func.
type liveSession struct{ Session }

// Connected reports whether a server has a session that has not died.
func (c *Catalog) Connected(server string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.sessions[server]
	return ok
}

// retireIfDead drops a server's session when err says its connection is gone.
//
// The tools stay: a reconnect replaces them, and until then a call fails as
// ErrUnknownServer — which is true — rather than as an unknown tool. Only the
// session that failed is retired; if a reconnect already installed another,
// that one is left alone.
func (c *Catalog) retireIfDead(server string, failed Session, err error) {
	if !errors.Is(err, mcp.ErrConnectionClosed) {
		return
	}
	c.mu.Lock()
	current, ok := c.sessions[server]
	if !ok || current != failed {
		c.mu.Unlock()
		return
	}
	delete(c.sessions, server)
	c.mu.Unlock()
	_ = failed.Close()
}
