package ledger

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

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

	// percentile_disc, not percentile_cont: a reported duration has to be one
	// some run actually had, and it has to agree with the in-memory ledger,
	// which cannot interpolate between two runs that never existed.
	var median, p95 *float64
	err = p.pool.QueryRow(ctx, `
		select count(*),
		       percentile_disc(0.5) within group (
		           order by extract(epoch from (ended_at - started_at)) * 1000
		       ),
		       percentile_disc(0.95) within group (
		           order by extract(epoch from (ended_at - started_at)) * 1000
		       )
		from runs `+whereAnd(where, "ended_at is not null"), args...,
	).Scan(&out.Ended, &median, &p95)
	if err != nil {
		return domain.RunStats{}, fmt.Errorf("stats durations: %w", err)
	}
	if median != nil {
		out.MedianDurationMS = int64(*median)
	}
	if p95 != nil {
		out.P95DurationMS = int64(*p95)
	}

	return out, nil
}

// Throughput buckets runs by the hour they started.
//
// date_trunc in the database rather than in Go: the alternative is reading
// every run in the window into the process to group it, which is the shape
// this projection exists to avoid.
func (p *Postgres) Throughput(ctx context.Context, filter domain.RunFilter) ([]domain.ThroughputBucket, error) {
	where, args := runFilterSQL(filter)

	rows, err := p.pool.Query(ctx, `
		select date_trunc('hour', started_at) as bucket, phase, agent_id,
		       count(*), coalesce(sum(cost_micros), 0)
		from runs `+where+`
		group by bucket, phase, agent_id
		order by bucket`, args...)
	if err != nil {
		return nil, fmt.Errorf("throughput: %w", err)
	}
	defer rows.Close()

	var out []domain.ThroughputBucket
	for rows.Next() {
		var at time.Time
		var phase, agent string
		var count, micros int64
		if err := rows.Scan(&at, &phase, &agent, &count, &micros); err != nil {
			return nil, err
		}
		out = appendBucket(out, at.UTC(), phase, agent, count, micros)
	}
	return out, rows.Err()
}

// appendBucket folds one (hour, phase, agent) tally into the ordered result.
//
// The query returns a row per phase per agent per hour; the caller wants one
// bucket per hour. Ordered by bucket, so only the last one can still be open.
func appendBucket(
	out []domain.ThroughputBucket, at time.Time, phase, agent string, count, micros int64,
) []domain.ThroughputBucket {
	if len(out) == 0 || !out[len(out)-1].At.Equal(at) {
		out = append(out, domain.ThroughputBucket{
			At: at, ByPhase: map[string]int64{}, ByAgent: map[string]int64{},
		})
	}
	b := &out[len(out)-1]
	b.ByPhase[phase] += count
	b.ByAgent[agent] += count
	b.Total += count
	b.Micros += micros
	return out
}

// Decisions reads the most recent Gate decisions across every matching run.
//
// Straight from the steps rather than from the runs projection: a decision is
// a step, and the projection holds one row per run. The index on (kind, at) is
// what keeps this from walking the whole ledger.
func (p *Postgres) Decisions(ctx context.Context, filter domain.RunFilter, limit int) ([]domain.RecordedDecision, error) {
	// The steps table times its rows with `at`; the runs projection uses
	// started_at. Same filter, different column, so it is named rather than
	// assumed.
	where, args := runFilterOn(filter, "at")
	where = whereAnd(where, "kind = 'gate_decided'")
	args = append(args, limit)

	rows, err := p.pool.Query(ctx, `
		select run_id, seq, at, company_id, area_id, agent_id,
		       payload->>'tool', coalesce((payload->>'verdict')::int, 0),
		       coalesce(payload->>'rule', ''), coalesce(payload->>'policy_code', '')
		from run_steps `+where+`
		order by at desc, seq desc
		limit $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("decisions: %w", err)
	}
	defer rows.Close()

	var out []domain.RecordedDecision
	for rows.Next() {
		var d domain.RecordedDecision
		var company, area, agent, tool string
		var verdict int
		if err := rows.Scan(&d.RunID, &d.Seq, &d.At, &company, &area, &agent,
			&tool, &verdict, &d.Rule, &d.PolicyCode); err != nil {
			return nil, err
		}
		d.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
		d.AgentID, d.Tool, d.Verdict = domain.AgentID(agent), domain.ToolID(tool), domain.Verdict(verdict)
		d.At = d.At.UTC()
		out = append(out, d)
	}
	return out, rows.Err()
}

// RunByIdemKey finds the run a key already opened.
//
// The unique index on idem_key is what makes the key a promise rather than a
// hope: a second attempt cannot land, and this is how the caller that made it
// discovers which run holds the first.
func (p *Postgres) RunByIdemKey(ctx context.Context, key string) (domain.RunID, error) {
	if key == "" {
		return "", ErrNotFound
	}
	var runID domain.RunID
	err := p.pool.QueryRow(ctx,
		`select run_id from run_steps where idem_key = $1`, key).Scan(&runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("run by idempotency key: %w", err)
	}
	return runID, nil
}

// runFilterSQL builds the shared predicate against the runs projection.
func runFilterSQL(f domain.RunFilter) (string, []any) {
	return runFilterOn(f, "started_at")
}

// runFilterOn builds the predicate against whichever column carries the time.
// Every argument is bound, never interpolated: a filter comes from a query
// string. The column is a literal chosen here, never a caller's input.
func runFilterOn(f domain.RunFilter, timeColumn string) (string, []any) {
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
	if len(f.Scopes) > 0 {
		var any []string
		for _, scope := range f.Scopes {
			args = append(args, string(scope.Company))
			company := fmt.Sprintf("company_id = $%d", len(args))
			if scope.Area == "" {
				// A grant with no area covers the whole company.
				any = append(any, company)
				continue
			}
			args = append(args, string(scope.Area))
			any = append(any, fmt.Sprintf("(%s and area_id = $%d)", company, len(args)))
		}
		clauses = append(clauses, "("+strings.Join(any, " or ")+")")
	}
	if !f.Since.IsZero() {
		add(timeColumn+" >= $%d", f.Since.UTC())
	}
	// Every figure that compares one window against another needs both ends.
	// Carrying Until and applying only Since made "yesterday" mean "yesterday
	// onwards", which includes the today it was being compared to.
	if !f.Until.IsZero() {
		add(timeColumn+" < $%d", f.Until.UTC())
	}
	if f.Search != "" {
		// One bound parameter used twice: the pattern is built here rather
		// than interpolated, so a search string cannot become SQL.
		args = append(args, "%"+f.Search+"%")
		clauses = append(clauses,
			fmt.Sprintf("(run_id ilike $%d or agent_id ilike $%d)", len(args), len(args)))
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
