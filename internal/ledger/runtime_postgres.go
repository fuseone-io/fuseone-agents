package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/mcpmetrics"
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
	since := runtimeWindowSince(filter.Since, time.Now().UTC())

	out := domain.RuntimeHealth{ByPhase: map[string]int64{}}

	where, args := runFilterSQL(current)
	where = whereAnd(where, realRuns)

	activeWhere := whereAnd(where, "phase in "+runtimeActivePhases)
	rows, err := p.pool.Query(ctx, `
		select phase, count(*)
		from runs `+activeWhere+`
		group by phase`, args...)
	if err != nil {
		return domain.RuntimeHealth{}, fmt.Errorf("runtime phases: %w", err)
	}
	if err := scanPhaseCounts(rows, out.ByPhase); err != nil {
		return domain.RuntimeHealth{}, err
	}

	terminalWhere := whereAnd(where, "phase in "+runtimeTerminalPhases)
	terminalWhere = whereAnd(terminalWhere, fmt.Sprintf("updated_at >= $%d", len(args)+1))
	terminalArgs := append(append([]any{}, args...), since)
	rows, err = p.pool.Query(ctx, `
		select phase, count(*)
		from runs `+terminalWhere+`
		group by phase`, terminalArgs...)
	if err != nil {
		return domain.RuntimeHealth{}, fmt.Errorf("runtime recent phases: %w", err)
	}
	if err := scanPhaseCounts(rows, out.ByPhase); err != nil {
		return domain.RuntimeHealth{}, err
	}

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
	recent.After = nil
	recent.Search = ""
	recent.Since = time.Time{}
	where, args = runFilterSQL(recent)
	where = whereAnd(whereAnd(where, realRuns), "failure_code is not null")
	where = whereAnd(where, fmt.Sprintf("updated_at >= $%d", len(args)+1))
	args = append(args, since)
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
	if err := rows.Err(); err != nil {
		return domain.RuntimeHealth{}, err
	}

	recent.Since = time.Time{}
	where, args = runFilterOn(recent, "at")
	where = whereAnd(whereAnd(where, realSteps), "kind = 'tool_returned'")
	where = whereAnd(where, "payload->>'failed' = 'true'")
	where = whereAnd(where, fmt.Sprintf("at >= $%d", len(args)+1))
	args = append(args, since, mcpmetrics.Codes())
	rows, err = p.pool.Query(ctx, `
		with failed as (
			select case
			         when coalesce(payload->>'error_code', '') = ''
			           then '`+mcpmetrics.CodeNoCode+`'
			         else coalesce(payload->>'error_code', '')
			       end as raw_code,
			       run_id, at
			from run_steps `+where+`
		)
		select case
		         when raw_code = any($`+fmt.Sprint(len(args))+`) then raw_code
		         else '`+mcpmetrics.CodeOther+`'
		       end as code,
		       count(*), count(distinct run_id), max(at)
		from failed
		group by code
		order by count(*) desc, max(at) desc`, args...)
	if err != nil {
		return domain.RuntimeHealth{}, fmt.Errorf("runtime tool failures: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var one domain.RuntimeToolFailureBucket
		if err := rows.Scan(&one.Code, &one.Calls, &one.Runs, &one.LastAt); err != nil {
			return domain.RuntimeHealth{}, err
		}
		one.LastAt = one.LastAt.UTC()
		out.ToolFailures = append(out.ToolFailures, one)
	}
	return out, rows.Err()
}

const (
	runtimeActivePhases   = "('running', 'awaiting_tool', 'awaiting_approval', 'parked', 'compensating')"
	runtimeTerminalPhases = "('finished', 'failed')"
)

func runtimeWindowSince(since, now time.Time) time.Time {
	if since.IsZero() {
		return now.Add(-24 * time.Hour)
	}
	return since
}

func scanPhaseCounts(rows pgxRows, into map[string]int64) error {
	defer rows.Close()
	for rows.Next() {
		var phase string
		var count int64
		if err := rows.Scan(&phase, &count); err != nil {
			return err
		}
		into[phase] += count
	}
	return rows.Err()
}

type pgxRows interface {
	Close()
	Next() bool
	Scan(...any) error
	Err() error
}
