package admin

import (
	"context"
	"encoding/json"
	"fmt"
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
		var stored storedMCPUserCredential
		if err := json.Unmarshal(row.Value, &stored); err != nil {
			return "", false, fmt.Errorf("%w: %s: %v", ErrMCPPersonalCredentialCorrupt, row.Name, err)
		}
		if !row.HasSecret || rowServer(stored, row.Name) != name || !sameServerRow(stored.serverRuntime(), at) {
			continue
		}
		who, when := domain.UserID(stored.Principal), stored.configuredOr(row.UpdatedAt)
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
