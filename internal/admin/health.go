package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// Health is what the platform observed about the systems it connects to.
//
// Distinct from the settings that configure them: configuration is what an
// operator wrote, observation is what happened when the platform tried. A
// screen that showed only the first would report a server as configured while
// it had been refusing connections for a week.
type Health struct{ pool *pgxpool.Pool }

func NewHealth(pool *pgxpool.Pool) *Health { return &Health{pool: pool} }

// Record stores what one attempt found, replacing the previous observation.
//
// The history of connections is not kept: a server that has flapped a thousand
// times is still one row, and what people did about it lives in the
// administrative trail where decisions belong.
func (h *Health) Record(ctx context.Context, obs domain.IntegrationHealth) error {
	_, err := h.pool.Exec(ctx, `
		insert into integration_health (name, reachable, tool_count, detail, observed_at, observed_by)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (name) do update set
			reachable = excluded.reachable,
			tool_count = excluded.tool_count,
			detail = excluded.detail,
			observed_at = excluded.observed_at,
			observed_by = excluded.observed_by`,
		obs.Name, obs.Reachable, obs.ToolCount, obs.Detail, obs.ObservedAt.UTC(), obs.ObservedBy)
	if err != nil {
		return fmt.Errorf("admin: record health of %s: %w", obs.Name, err)
	}
	return nil
}

// All returns the latest observation of every server, by name.
func (h *Health) All(ctx context.Context) (map[string]domain.IntegrationHealth, error) {
	rows, err := h.pool.Query(ctx, `
		select name, reachable, tool_count, detail, observed_at, observed_by
		from integration_health`)
	if err != nil {
		return nil, fmt.Errorf("admin: read health: %w", err)
	}
	defer rows.Close()

	out := map[string]domain.IntegrationHealth{}
	for rows.Next() {
		var obs domain.IntegrationHealth
		var observed time.Time
		if err := rows.Scan(&obs.Name, &obs.Reachable, &obs.ToolCount,
			&obs.Detail, &observed, &obs.ObservedBy); err != nil {
			return nil, err
		}
		obs.ObservedAt = observed.UTC()
		out[obs.Name] = obs
	}
	return out, rows.Err()
}
