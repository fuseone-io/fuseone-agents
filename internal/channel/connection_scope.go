package channel

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

// ErrConnectionScopeUnknown is a connection row of no scope this binary knows.
var ErrConnectionScopeUnknown = errors.New("channel: a connection stored at a scope this binary does not know")

/*
Via is the connection an event arrived on, with the scope the session that
received it held: the HTTP door's verifying read, or the Socket Mode session as
it was opened.

Judged by this and never by the connection's row read again later. Between the
event and the judgement the row can be removed, or recreated under the same
name at another scope; the event still came from the app that held the old one,
and is judged as that app's. A Via with no scope carries nothing.
*/
type Via struct {
	Name  string
	Scope domain.Scope
}

// carries answers whether an arrival through this connection may reach a
// conversation or a ticket stored at scope.
//
// A Via nobody gave a scope reaches nothing, and containment alone does not
// say so: a row stored at the installation has the zero scope, and the zero
// scope contains itself. Without the first condition an arrival whose door
// forgot the scope would reach every installation-wide conversation.
func (v Via) carries(scope domain.Scope) bool {
	return v.Scope != (domain.Scope{}) && v.Scope.Contains(scope)
}

// Held is a connection's credentials with the scope the connection serves: the
// scope a run, a conversation or a decision must be inside to use it.
type Held struct {
	Credentials Credentials
	Scope       domain.Scope
}

/*
ConnectionScope is the scope a conversation must be inside to be carried by a
connection row (NT-019, phase 4 readers), and whether this binary knows it at
all.

Only the three scopes, each with exactly the columns it means. A kind a later
release adds, or columns that contradict the kind, are no scope here: read as
some scope, the row's token would speak, and its signing secret would admit,
for runs it was never meant for.
*/
func ConnectionScope(kind settings.ScopeKind, stored domain.Scope) (domain.Scope, bool) {
	// The installation's sentinel contains every scope: as a company, or an
	// area's company, it would carry every conversation.
	named := stored.Company != "" && stored.Company != domain.Installation
	switch {
	case kind == settings.ScopeInstallation && stored == (domain.Scope{}):
		return domain.Scope{Company: domain.Installation}, true
	case kind == settings.ScopeCompany && named && stored.Area == "":
		return stored, true
	case kind == settings.ScopeArea && named && stored.Area != "":
		return stored, true
	}
	return domain.Scope{}, false
}

// RefuseUnknownConnectionScope is the allow a read by name opens a connection
// with: one of no known scope is not found, and its credentials are never
// opened.
func RefuseUnknownConnectionScope(kind settings.ScopeKind, stored domain.Scope) error {
	if _, known := ConnectionScope(kind, stored); !known {
		return ErrConnectionScopeUnknown
	}
	return nil
}

/*
carriers is where each connection is stored, read once for one delivery.

Delivery only. An arrival is judged by the Via it came through, never by the
row read again, because the row may have changed since the event was received.

A connection that is not stored at all is not judged here. A conversation may be
configured before its connection, and nothing can be posted to or arrive from a
connection that does not exist: delivery reads the connection by name and
fails, loudly, as it always has. A connection stored at a scope nobody knows
carries nothing.
*/
type carriers struct {
	serves  map[string]domain.Scope
	unknown map[string]bool
}

func (c *Configured) carriers(ctx context.Context) (carriers, error) {
	stored, err := c.store.List(ctx, KindChannel)
	if err != nil {
		return carriers{}, fmt.Errorf("channel: list connections: %w", err)
	}
	return carriersOf(stored), nil
}

func carriersOf(stored []settings.Setting) carriers {
	out := carriers{serves: map[string]domain.Scope{}, unknown: map[string]bool{}}
	for _, s := range stored {
		if at, known := ConnectionScope(s.ScopeKind, s.Scope); known {
			out.serves[s.Name] = at
			continue
		}
		out.unknown[s.Name] = true
	}
	return out
}

// carries answers whether a conversation stored at scope may be reached through
// the connection it names.
func (c carriers) carries(connection string, scope domain.Scope) bool {
	if c.unknown[connection] {
		return false
	}
	at, stored := c.serves[connection]
	return !stored || at.Contains(scope)
}
