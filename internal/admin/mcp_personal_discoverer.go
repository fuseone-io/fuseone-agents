package admin

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
MCPPersonalDiscoverer is the person a server that takes each person's own
credential is discovered as, when nothing else can discover it: whoever
configured their credential for that row of the server last, the smaller
principal on a tie. Every worker reads the same rows and picks the same person.

There is no failover between people. The one picked may hold a credential the
server refuses, and the server then stays unreachable until that person
connects again or removes theirs, even when somebody earlier holds one that
works: an order that skipped the failing would make which account the catalogue
was learned from depend on which worker tried whom first.

Unreadable rows are an error, never nobody: "waiting for a person" would be
the wrong thing to tell somebody who is already connected.
*/
func (i *Integrations) MCPPersonalDiscoverer(
	ctx context.Context, at domain.Scope, name string,
) (domain.UserID, bool, error) {
	rows, err := i.settings.ListUnderPrefixTx(ctx, i.pool, settings.KindMCPUserCredential, encodeSettingPart(name)+".")
	if err != nil {
		return "", false, fmt.Errorf("admin: list personal credentials of %s: %w", name, err)
	}
	var (
		best   domain.UserID
		bestAt time.Time
	)
	for _, row := range rows {
		// The value has to name the person and the server its key does, as
		// when one person's credential is read: one naming another would be
		// discovered as whoever the value says.
		who := rowPrincipal(row.Name)
		if who == "" {
			return "", false, fmt.Errorf("%w: %s", ErrMCPPersonalCredentialCorrupt, row.Name)
		}
		stored, err := storedPersonalCredential(row, name, who)
		if err != nil {
			return "", false, err
		}
		if !row.HasSecret || !sameServerRow(stored.serverRuntime(), at) {
			continue
		}
		when := stored.configuredOr(row.UpdatedAt)
		if best == "" || when.After(bestAt) || (when.Equal(bestAt) && who < best) {
			best, bestAt = who, when
		}
	}
	return best, best != "", nil
}

// configuredOr is when the person configured the credential; a row stored
// before that was recorded has only when it was written.
func (c storedMCPUserCredential) configuredOr(written time.Time) time.Time {
	if c.ConfiguredAt.IsZero() {
		return written
	}
	return c.ConfiguredAt
}

// MCPDiscoveredOnBehalf is one worker having discovered a server as a person.
type MCPDiscoveredOnBehalf struct {
	Server string
	// Scope is where the server row is stored (NT-019).
	Scope     domain.Scope
	Principal domain.UserID
	// Holder is the worker, as it names itself in its publications.
	Holder string
}

// RecordMCPDiscoveredOnBehalf records which credential a worker learned the
// server's catalogue with, by ids alone. One worker's record: every worker, and
// every restart of one, discovers on its own and says so.
func (i *Integrations) RecordMCPDiscoveredOnBehalf(ctx context.Context, d MCPDiscoveredOnBehalf) error {
	return Record(ctx, i.pool, Event{
		Principal: domain.SystemWorker, Scope: mcpServerRuntimeScope(storedAt(d.Scope)),
		Action: "mcp_server.discovered_on_behalf", Target: d.Server,
		Detail: map[string]string{"principal": string(d.Principal), "holder": d.Holder},
	})
}

// rowPrincipal is the person a credential's key names; nobody when the key
// does not decode.
func rowPrincipal(name string) domain.UserID {
	_, encoded, ok := strings.Cut(name, ".")
	if !ok {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return ""
	}
	return domain.UserID(raw)
}
