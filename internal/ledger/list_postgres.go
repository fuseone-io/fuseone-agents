package ledger

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// runColumns is the projection read back in the order Scan expects.
const runColumns = `
	run_id, company_id, area_id, agent_id, version_id, on_behalf_of,
	phase, last_seq,
	cost_micros, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
	reserved_micros, tool_calls, labels,
	pending_tool, pending_rule, pending_reason, pending_at_seq,
	started_at, ended_at, updated_at,
	failure_code, failure_provider, failure_status, failure_request_id, failure_retryable`

// ListRuns reads a page from the projection.
//
// Newest first, and the filter is applied by the database. The alternative —
// reading every run and folding it until enough match — makes the console's
// cost grow with the installation's history.
func (p *Postgres) ListRuns(ctx context.Context, filter domain.RunFilter, phase string, limit int) ([]domain.RunSummary, error) {
	where, args := runFilterSQL(filter)
	where = whereAnd(where, realRuns)
	if phase != "" {
		args = append(args, phase)
		where = whereAnd(where, fmt.Sprintf("phase = $%d", len(args)))
	}
	if c := filter.After; c != nil {
		// Compared as a tuple, in the same order the list is read in, so the
		// boundary is one place rather than three cases.
		args = append(args, c.StartedAt.UTC(), string(c.RunID))
		where = whereAnd(where, fmt.Sprintf(
			"(started_at, run_id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, limitOrDefault(limit))

	rows, err := p.pool.Query(ctx, `select`+runColumns+` from runs `+where+
		fmt.Sprintf(` order by started_at desc, run_id desc limit $%d`, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	var out []domain.RunSummary
	for rows.Next() {
		summary, err := scanRunSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, summary)
	}
	return out, rows.Err()
}

// CostRollup sums in the database, grouped by whichever dimension was asked
// for. Reading every step of every run to add up money is how a cost page
// becomes the most expensive thing an installation does.
func (p *Postgres) CostRollup(ctx context.Context, filter domain.RunFilter, groupBy string) ([]domain.CostBucket, error) {
	column, err := rollupColumn(groupBy)
	if err != nil {
		return nil, err
	}

	where, args := runFilterSQL(filter)
	where = whereAnd(where, realRuns)
	if filter.Until.IsZero() {
		return nil, fmt.Errorf("ledger: a cost rollup needs an upper bound")
	}
	args = append(args, filter.Until.UTC())
	where = whereAnd(where, fmt.Sprintf("started_at <= $%d", len(args)))

	rows, err := p.pool.Query(ctx, `
		select `+column+` as bucket,
		       count(*),
		       coalesce(sum(cost_micros), 0),
		       coalesce(sum(input_tokens), 0),
		       coalesce(sum(output_tokens), 0),
		       coalesce(sum(cache_read_tokens), 0),
		       coalesce(sum(cache_write_tokens), 0)
		from runs `+where+`
		group by bucket order by bucket`, args...)
	if err != nil {
		return nil, fmt.Errorf("cost rollup: %w", err)
	}
	defer rows.Close()

	var out []domain.CostBucket
	for rows.Next() {
		var b domain.CostBucket
		if err := rows.Scan(&b.Key, &b.Runs, &b.Cost.Micros,
			&b.Cost.InputTokens, &b.Cost.OutputTokens,
			&b.Cost.CacheReadTokens, &b.Cost.CacheWriteTokens); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// rollupColumn maps the contract's dimension onto a column. It is a closed set
// rather than a string the caller supplies, because this value is interpolated
// into SQL and a query parameter cannot stand in for a column name.
func rollupColumn(groupBy string) (string, error) {
	switch groupBy {
	case "", "agent":
		return "agent_id", nil
	case "area":
		return "area_id", nil
	case "company":
		return "company_id", nil
	case "day":
		return "to_char(started_at, 'YYYY-MM-DD')", nil
	}
	return "", fmt.Errorf("ledger: cannot group cost by %q", groupBy)
}

func limitOrDefault(limit int) int {
	if limit <= 0 || limit > 500 {
		return 50
	}
	return limit
}
