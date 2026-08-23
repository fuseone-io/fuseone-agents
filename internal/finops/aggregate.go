package finops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
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
func (s *Spend) ByModel(ctx context.Context, filter domain.RunFilter) ([]Bucket, error) {
	return s.aggregate(ctx, filter, "provider, model", "provider, model, ''")
}

// ByAgent sums the same window by which agent spent it.
func (s *Spend) ByAgent(ctx context.Context, filter domain.RunFilter) ([]Bucket, error) {
	return s.aggregate(ctx, filter, "agent_id", "'', '', agent_id")
}

// ProjectedFrom is when this projection started making claims.
func (s *Spend) ProjectedFrom(ctx context.Context) (time.Time, error) {
	var at time.Time
	err := s.pool.QueryRow(ctx, `
		select started_at from planning_spend_cursor where id = true`).Scan(&at)
	if err == nil {
		return at.UTC(), nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	return time.Time{}, fmt.Errorf("finops: read spend projection start: %w", err)
}

// aggregate runs one shape of rollup. `groupBy` and `selected` are constants
// from the two callers above and never reach here from a request.
func (s *Spend) aggregate(ctx context.Context, filter domain.RunFilter, groupBy, selected string) ([]Bucket, error) {
	where, args, err := planningSpendFilterSQL(filter)
	if err != nil {
		return nil, err
	}
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
		from planning_spend `+where+`
		group by `+groupBy+`
		order by coalesce(sum(cost_micros), 0) desc, count(*) desc`, args...)
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

func planningSpendFilterSQL(f domain.RunFilter) (string, []any, error) {
	if f.Until.IsZero() {
		return "", nil, fmt.Errorf("finops: a planning spend rollup needs an upper bound")
	}

	var (
		clauses []string
		args    []any
	)
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}

	if !f.Since.IsZero() {
		add("day >= ($%d at time zone 'UTC')::date", f.Since.UTC())
	}
	add("day <= ($%d at time zone 'UTC')::date", f.Until.UTC())
	if f.AgentID != "" {
		add("agent_id = $%d", string(f.AgentID))
	}
	if f.Scope.Company != "" {
		addScope(&clauses, &args, f.Scope)
	}
	if len(f.Scopes) > 0 {
		if !reachesEveryScope(f.Scopes) {
			var any []string
			for _, scope := range f.Scopes {
				if clause := scopePredicate(&args, scope); clause != "" {
					any = append(any, clause)
				}
			}
			if len(any) > 0 {
				clauses = append(clauses, "("+strings.Join(any, " or ")+")")
			}
		}
	}

	return "where " + strings.Join(clauses, " and "), args, nil
}

func reachesEveryScope(scopes []domain.Scope) bool {
	for _, scope := range scopes {
		if scope.Company == domain.Installation && scope.Area == "" {
			return true
		}
	}
	return false
}

func addScope(clauses *[]string, args *[]any, scope domain.Scope) {
	clause := scopePredicate(args, scope)
	if clause != "" {
		*clauses = append(*clauses, clause)
	}
}

func scopePredicate(args *[]any, scope domain.Scope) string {
	if scope.Company == domain.Installation && scope.Area == "" {
		return ""
	}
	*args = append(*args, string(scope.Company))
	company := fmt.Sprintf("company_id = $%d", len(*args))
	if scope.Area == "" {
		return company
	}
	*args = append(*args, string(scope.Area))
	return fmt.Sprintf("(%s and area_id = $%d)", company, len(*args))
}
