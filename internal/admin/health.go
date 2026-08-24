package admin

import (
	"context"
	"database/sql"
	"fmt"

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
		insert into integration_health (
			name, reachable, tool_count, detail, observed_at, observed_by,
			last_reachable_at
		)
		values ($1, $2, $3, $4, $5, $6,
			case when $2::boolean then $5::timestamptz else null::timestamptz end)
		on conflict (name) do update set
			reachable = excluded.reachable,
			tool_count = excluded.tool_count,
			detail = excluded.detail,
			observed_at = excluded.observed_at,
			observed_by = excluded.observed_by,
			last_reachable_at = case
				when excluded.reachable then excluded.observed_at
				else integration_health.last_reachable_at
			end`,
		obs.Name, obs.Reachable, obs.ToolCount, obs.Detail, obs.ObservedAt.UTC(), obs.ObservedBy)
	if err != nil {
		return fmt.Errorf("admin: record health of %s: %w", obs.Name, err)
	}
	return nil
}

// RecordToolCall stores what the last tools/call attempt proved.
//
// It updates only the call half. Discovery health is written by the reconciler,
// and letting a runtime call overwrite it would turn "discovery works but calls
// fail" back into one vague red light.
func (h *Health) RecordToolCall(ctx context.Context, obs domain.IntegrationToolCallObservation) error {
	tag, err := h.pool.Exec(ctx, `
		update integration_health set
			tool_call_ok = $2,
			tool_call_code = $3,
			tool_call_observed_at = $4,
			tool_call_observed_by = $5,
			last_tool_call_ok_at = case
				when $2::boolean then $4::timestamptz
				else integration_health.last_tool_call_ok_at
			end
		where name = $1`,
		obs.Name, obs.OK, obs.Code, obs.ObservedAt.UTC(), obs.ObservedBy)
	if err != nil {
		return fmt.Errorf("admin: record tool-call health of %s: %w", obs.Name, err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	return nil
}

// Forget drops what was observed about a server.
//
// Called when one is removed, because the screen shows configuration and
// observation together: an observation left behind renders as a server nobody
// configured, which cannot be edited and cannot be deleted. A deletion that
// leaves a row nobody can act on is not a deletion.
//
// Forgetting one nobody observed is not an error: removal is asked from the
// configured set, not from knowledge of what was ever reached.
func (h *Health) Forget(ctx context.Context, name string) error {
	if _, err := h.pool.Exec(ctx,
		`delete from integration_health where name = $1`, name); err != nil {
		return fmt.Errorf("admin: forget health of %s: %w", name, err)
	}
	return nil
}

// All returns the latest observation of every server, by name.
func (h *Health) All(ctx context.Context) (map[string]domain.IntegrationHealth, error) {
	rows, err := h.pool.Query(ctx, `
		select name, reachable, tool_count, detail, observed_at, observed_by,
		       last_reachable_at, tool_call_ok, tool_call_code,
		       tool_call_observed_at, tool_call_observed_by,
		       last_tool_call_ok_at
		from integration_health`)
	if err != nil {
		return nil, fmt.Errorf("admin: read health: %w", err)
	}
	defer rows.Close()

	out := map[string]domain.IntegrationHealth{}
	for rows.Next() {
		obs, err := scanHealth(rows)
		if err != nil {
			return nil, err
		}
		out[obs.Name] = obs
	}
	return out, rows.Err()
}

type healthRow interface {
	Scan(dest ...any) error
}

func scanHealth(row healthRow) (domain.IntegrationHealth, error) {
	var obs domain.IntegrationHealth
	var observed, lastReachable sql.NullTime
	var callOK sql.NullBool
	var callCode, callBy string
	var callObserved, lastCallOK sql.NullTime
	err := row.Scan(&obs.Name, &obs.Reachable, &obs.ToolCount,
		&obs.Detail, &observed, &obs.ObservedBy, &lastReachable,
		&callOK, &callCode, &callObserved, &callBy, &lastCallOK)
	if err != nil {
		return domain.IntegrationHealth{}, err
	}
	obs.ObservedAt = observed.Time.UTC()
	if lastReachable.Valid {
		at := lastReachable.Time.UTC()
		obs.LastReachableAt = &at
	}
	obs.ToolCall = toolCallHealth(callOK, callCode, callBy, callObserved, lastCallOK)
	return obs, nil
}

func toolCallHealth(
	ok sql.NullBool,
	code, by string,
	observed, lastOK sql.NullTime,
) *domain.IntegrationToolCallHealth {
	if !ok.Valid || !observed.Valid {
		return nil
	}
	out := &domain.IntegrationToolCallHealth{
		OK: ok.Bool, Code: code, ObservedAt: observed.Time.UTC(), ObservedBy: by,
	}
	if lastOK.Valid {
		at := lastOK.Time.UTC()
		out.LastOKAt = &at
	}
	return out
}
