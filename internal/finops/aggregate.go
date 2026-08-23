package finops

import (
	"context"
	"fmt"
	"time"
)

/*
Bucket is what one model, or one agent, spent over a window.

Unpriced travels with the money on purpose. A bucket whose calls had no
configured rate has real tokens and a cost of zero, and a total that folded
those in silently would read as cheap rather than as unknown — the same
confusion the run screen exists to end, one level up.
*/
type Bucket struct {
	Provider string
	Model    string
	Agent    string

	Calls            int64
	Runs             int64
	Micros           int64
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	// Unpriced is how many of Calls had no configured rate behind them.
	Unpriced int64
}

// ByModel sums the window by the pair a call was actually made against.
//
// Reads the projection, never the chain: this is opened when somebody is
// worried about money, which is the worst moment to make the database walk an
// append-only log.
func (s *Spend) ByModel(ctx context.Context, from, to time.Time) ([]Bucket, error) {
	return s.aggregate(ctx, from, to, "provider, model", "provider, model, ''")
}

// ByAgent sums the same window by which agent spent it.
func (s *Spend) ByAgent(ctx context.Context, from, to time.Time) ([]Bucket, error) {
	return s.aggregate(ctx, from, to, "agent_id", "'', '', agent_id")
}

// aggregate runs one shape of rollup. `groupBy` and `selected` are constants
// from the two callers above and never reach here from a request.
func (s *Spend) aggregate(ctx context.Context, from, to time.Time, groupBy, selected string) ([]Bucket, error) {
	rows, err := s.pool.Query(ctx, `
		select `+selected+`,
		       count(*) as calls,
		       count(distinct run_id) as runs,
		       coalesce(sum(cost_micros), 0),
		       coalesce(sum(input_tokens), 0),
		       coalesce(sum(output_tokens), 0),
		       coalesce(sum(cache_read_tokens), 0),
		       coalesce(sum(cache_write_tokens), 0),
		       count(*) filter (where price_status <> 'configured') as unpriced
		from planning_spend
		where day >= $1::date and day <= $2::date
		group by `+groupBy+`
		order by coalesce(sum(cost_micros), 0) desc, count(*) desc`,
		from.UTC(), to.UTC())
	if err != nil {
		return nil, fmt.Errorf("finops: aggregate spend: %w", err)
	}
	defer rows.Close()

	var out []Bucket
	for rows.Next() {
		var b Bucket
		if err := rows.Scan(&b.Provider, &b.Model, &b.Agent,
			&b.Calls, &b.Runs, &b.Micros,
			&b.InputTokens, &b.OutputTokens, &b.CacheReadTokens, &b.CacheWriteTokens,
			&b.Unpriced); err != nil {
			return nil, fmt.Errorf("finops: scan bucket: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
