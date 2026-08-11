package ledger

import (
	"context"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

// Stats aggregates in the database rather than in Go.
//
// The materialised projection exists for exactly this: answering a question
// about every run without reading every step of every run. Pulling the ledger
// into the process to count it would make the console's cost grow with the
// installation's history, which is the one thing an audit trail is guaranteed
// to accumulate.
func (p *Postgres) Stats(ctx context.Context, filter domain.RunFilter) (domain.RunStats, error) {
	where, args := runFilterSQL(filter)

	out := domain.RunStats{ByPhase: map[string]int64{}}

	rows, err := p.pool.Query(ctx,
		`select phase, count(*) from runs `+where+` group by phase`, args...)
	if err != nil {
		return domain.RunStats{}, fmt.Errorf("stats by phase: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var phase string
		var count int64
		if err := rows.Scan(&phase, &count); err != nil {
			return domain.RunStats{}, err
		}
		out.ByPhase[phase] = count
		out.Total += count
	}
	if err := rows.Err(); err != nil {
		return domain.RunStats{}, err
	}

	// percentile_disc, not percentile_cont: the median has to be a duration
	// some run actually had, and it has to agree with the in-memory ledger,
	// which cannot interpolate between two runs that never existed.
	var median *float64
	err = p.pool.QueryRow(ctx, `
		select count(*),
		       percentile_disc(0.5) within group (
		           order by extract(epoch from (ended_at - started_at)) * 1000
		       )
		from runs `+whereAnd(where, "ended_at is not null"), args...,
	).Scan(&out.Ended, &median)
	if err != nil {
		return domain.RunStats{}, fmt.Errorf("stats median: %w", err)
	}
	if median != nil {
		out.MedianDurationMS = int64(*median)
	}

	return out, nil
}

// runFilterSQL builds the shared predicate. Every argument is bound, never
// interpolated: a filter comes from a query string.
func runFilterSQL(f domain.RunFilter) (string, []any) {
	var (
		clauses []string
		args    []any
	)
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}

	if f.Scope.Company != "" {
		add("company_id = $%d", string(f.Scope.Company))
	}
	if f.Scope.Area != "" {
		add("area_id = $%d", string(f.Scope.Area))
	}
	if f.AgentID != "" {
		add("agent_id = $%d", string(f.AgentID))
	}
	if !f.Since.IsZero() {
		add("started_at >= $%d", f.Since.UTC())
	}

	if len(clauses) == 0 {
		return "", nil
	}
	return "where " + strings.Join(clauses, " and "), args
}

func whereAnd(where, clause string) string {
	if where == "" {
		return "where " + clause
	}
	return where + " and " + clause
}
