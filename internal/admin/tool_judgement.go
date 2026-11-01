package admin

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

// judged is what of a publication the catalogue records: the servers it is
// the authority on, their tools, and which of them the configuration does
// not hold.
type judged struct {
	servers   []string
	entries   []domain.ToolEntry
	unmanaged map[string]bool
}

/*
judge keeps what a publication may say about each server it names.

A managed server is the authority on its tools only while the configuration
it was discovered under is the one in force: present, switched on, and not
changed since — the row's updated_at moves with every save, credentials and
toolsets included. A discovery of anything else describes a server that no
longer exists in that form. Absent or switched off, the sweep clears it;
changed, the worker that holds the current configuration speaks for it, and
this publication's word is dropped.

An unmanaged server — started from the command line — has no configuration to
be judged by, so its own worker's discovery is the authority. Unless a
configured server now carries the name: then the configuration decides.
*/
func judge(pub domain.ToolPublication, configured map[string]time.Time) judged {
	out := judged{unmanaged: map[string]bool{}}
	authority := map[string]bool{}
	for _, server := range pub.Servers {
		generation, managed := configured[server.Name]
		switch {
		case server.Unmanaged && !managed:
			out.unmanaged[server.Name] = true
		case server.Unmanaged, !managed, !generation.Equal(server.Generation):
			continue
		}
		authority[server.Name] = true
	}
	out.servers = slices.Sorted(maps.Keys(authority))
	for _, e := range pub.Entries {
		if authority[e.Server] {
			out.entries = append(out.entries, e)
		}
	}
	return out
}

// configuredServers is every server the configuration wants now — present and
// switched on — with when its configuration last changed.
func configuredServers(ctx context.Context, tx pgx.Tx) (map[string]time.Time, error) {
	rows, err := tx.Query(ctx, `
		select name, updated_at from settings
		where scope_kind = 'installation' and company_id = '' and area_id = ''
		  and kind = $1 and enabled`, string(settings.KindMCPServer))
	if err != nil {
		return nil, fmt.Errorf("admin: read the configured servers: %w", err)
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var (
			name    string
			changed time.Time
		)
		if err := rows.Scan(&name, &changed); err != nil {
			return nil, fmt.Errorf("admin: read the configured servers: %w", err)
		}
		out[name] = changed
	}
	return out, rows.Err()
}

// sweepUnconfigured drops the tools of every managed server the
// configuration no longer wants, whoever published them and whenever. It is
// what makes a lost or late publication harmless: the next one, from any
// worker, takes them away.
func sweepUnconfigured(ctx context.Context, tx pgx.Tx, configured map[string]time.Time) error {
	if _, err := tx.Exec(ctx, `
		delete from settings
		where scope_kind = 'installation' and company_id = '' and area_id = ''
		  and kind = $1
		  and not coalesce((value->>'unmanaged')::boolean, false)
		  and not (value->>'server' = any($2))`,
		// Never nil: "= any(NULL)" is NULL, and with nothing configured the
		// sweep would keep everything it exists to clear.
		toolKind, append([]string{}, slices.Collect(maps.Keys(configured))...)); err != nil {
		return fmt.Errorf("admin: drop the tools of servers no longer configured: %w", err)
	}
	return nil
}
