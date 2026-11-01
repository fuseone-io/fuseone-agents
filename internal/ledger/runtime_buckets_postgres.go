package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/channelmetrics"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/egressmetrics"
)

// The channel and installation-wide buckets of the runtime view, each read
// from its own projection rather than from runs.

func (p *Postgres) runtimeEgressDenials(
	ctx context.Context, since time.Time,
) ([]domain.RuntimeEgressDenialBucket, error) {
	args := []any{since, egressmetrics.Codes()}
	rows, err := p.pool.Query(ctx, `
		with denials as (
			select case
			         when code = any($2) then code
			         else '`+egressmetrics.CodeOther+`'
			       end as code,
			       server, host, port, attempts, first_seen, last_seen
			from mcp_egress_denials
			where last_seen >= $1
		)
		select code, coalesce(sum(attempts), 0), count(distinct server),
		       count(distinct (host, port)) filter (where host <> '' and port > 0),
		       min(first_seen), max(last_seen)
		from denials
		group by code
		order by coalesce(sum(attempts), 0) desc, max(last_seen) desc`, args...)
	if err != nil {
		return nil, fmt.Errorf("runtime stdio egress denials: %w", err)
	}
	return scanRuntimeEgressDenials(rows)
}

func scanRuntimeEgressDenials(rows pgxRows) ([]domain.RuntimeEgressDenialBucket, error) {
	defer rows.Close()
	var out []domain.RuntimeEgressDenialBucket
	for rows.Next() {
		var one domain.RuntimeEgressDenialBucket
		if err := rows.Scan(&one.Code, &one.Attempts, &one.Servers,
			&one.Destinations, &one.FirstAt, &one.LastAt); err != nil {
			return nil, err
		}
		one.FirstAt, one.LastAt = one.FirstAt.UTC(), one.LastAt.UTC()
		out = append(out, one)
	}
	return out, rows.Err()
}

func (p *Postgres) runtimeChannelFailures(
	ctx context.Context, filter domain.RunFilter, since time.Time,
) ([]domain.RuntimeChannelFailureBucket, error) {
	where, args := runFilterOn(filter, "last_seen")
	where = whereAnd(where, fmt.Sprintf("last_seen >= $%d", len(args)+1))
	args = append(args, since, channelmetrics.Codes())
	rows, err := p.pool.Query(ctx, `
		with failures as (
			select case
			         when code = any($`+fmt.Sprint(len(args))+`) then code
			         else '`+channelmetrics.CodeOther+`'
			       end as code,
			       run_id, channel, conversation, scope_wide, attempts, first_seen, last_seen
			from channel_delivery_failures `+where+`
		)
		select code, coalesce(sum(attempts), 0),
		       count(distinct (channel, conversation)) filter (where not scope_wide),
		       bool_or(scope_wide), count(distinct run_id),
		       min(first_seen), max(last_seen)
		from failures
		group by code
		order by coalesce(sum(attempts), 0) desc, max(last_seen) desc`, args...)
	if err != nil {
		return nil, fmt.Errorf("runtime channel failures: %w", err)
	}
	return scanRuntimeChannelFailures(rows)
}

func scanRuntimeChannelFailures(rows pgxRows) ([]domain.RuntimeChannelFailureBucket, error) {
	defer rows.Close()
	var out []domain.RuntimeChannelFailureBucket
	for rows.Next() {
		var one domain.RuntimeChannelFailureBucket
		if err := rows.Scan(&one.Code, &one.Attempts, &one.Conversations,
			&one.ScopeWide, &one.Runs, &one.FirstAt, &one.LastAt); err != nil {
			return nil, err
		}
		one.FirstAt, one.LastAt = one.FirstAt.UTC(), one.LastAt.UTC()
		out = append(out, one)
	}
	return out, rows.Err()
}
