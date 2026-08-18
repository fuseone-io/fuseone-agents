package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// RuntimeHealth reads the current worker queue and recent typed failures from
// the projection. It never folds run_steps: this page is opened during outages,
// when making the database do archival work would be a particularly poor idea.
func (p *Postgres) RuntimeHealth(ctx context.Context, filter domain.RunFilter) (domain.RuntimeHealth, error) {
	current := filter
	current.Since = time.Time{}
	current.Until = time.Time{}
	current.After = nil
	current.Search = ""

	out := domain.RuntimeHealth{ByPhase: map[string]int64{}}

	where, args := runFilterSQL(current)
	where = whereAnd(where, realRuns)
	rows, err := p.pool.Query(ctx, `
		select phase, count(*)
		from runs `+where+`
		group by phase`, args...)
	if err != nil {
		return domain.RuntimeHealth{}, fmt.Errorf("runtime phases: %w", err)
	}
	for rows.Next() {
		var phase string
		var count int64
		if err := rows.Scan(&phase, &count); err != nil {
			rows.Close()
			return domain.RuntimeHealth{}, err
		}
		out.ByPhase[phase] = count
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.RuntimeHealth{}, err
	}
	rows.Close()

	var oldest *time.Time
	if err := p.pool.QueryRow(ctx, `
		select
			count(*) filter (
				where phase in `+claimablePhases+`
				  and next_attempt_at <= now()
				  and (leased_until is null or leased_until <= now())
			),
			count(*) filter (
				where phase in `+claimablePhases+`
				  and leased_until > now()
			),
			count(*) filter (
				where phase in `+claimablePhases+`
				  and next_attempt_at > now()
			),
			count(*) filter (
				where phase in `+claimablePhases+`
				  and leased_until is not null
				  and leased_until <= now()
			),
			min(next_attempt_at) filter (
				where phase in `+claimablePhases+`
				  and next_attempt_at <= now()
				  and (leased_until is null or leased_until <= now())
			)
		from runs `+where,
		args...,
	).Scan(&out.Queue.Ready, &out.Queue.Leased, &out.Queue.BackingOff, &out.Queue.ExpiredLeases, &oldest); err != nil {
		return domain.RuntimeHealth{}, fmt.Errorf("runtime queue: %w", err)
	}
	if oldest != nil {
		out.Queue.OldestReadyAt = oldest.UTC()
	}

	recent := filter
	failureSince := recent.Since
	recent.After = nil
	recent.Search = ""
	recent.Since = time.Time{}
	if failureSince.IsZero() {
		failureSince = time.Now().UTC().Add(-24 * time.Hour)
	}
	where, args = runFilterSQL(recent)
	where = whereAnd(whereAnd(where, realRuns), "failure_code is not null")
	where = whereAnd(where, fmt.Sprintf("updated_at >= $%d", len(args)+1))
	args = append(args, failureSince)
	rows, err = p.pool.Query(ctx, `
		select failure_code, coalesce(failure_provider, ''), coalesce(failure_status, 0),
		       coalesce(failure_retryable, false), count(*), max(updated_at)
		from runs `+where+`
		group by failure_code, failure_provider, failure_status, failure_retryable
		order by count(*) desc, max(updated_at) desc`, args...)
	if err != nil {
		return domain.RuntimeHealth{}, fmt.Errorf("runtime failures: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var one domain.RuntimeFailureBucket
		if err := rows.Scan(
			&one.Code, &one.Provider, &one.Status, &one.Retryable, &one.Runs, &one.LastAt,
		); err != nil {
			return domain.RuntimeHealth{}, err
		}
		one.LastAt = one.LastAt.UTC()
		out.Failures = append(out.Failures, one)
	}
	return out, rows.Err()
}
