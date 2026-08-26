package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

func (e *Erasures) eraseMemoryRows(ctx context.Context, conn db, before time.Time) (int, error) {
	total := int64(0)
	for _, stmt := range memoryErasureStatements() {
		tag, err := conn.Exec(ctx, stmt, before.UTC(), retentionMemoryBatch)
		if err != nil {
			return int(total), fmt.Errorf("admin: erase memory records: %w", err)
		}
		total += tag.RowsAffected()
	}
	return int(total), nil
}

func memoryErasureStatements() []string {
	return []string{
		`with doomed as (
			select ctid from memory_assertion_events
			where at < $1
			order by at
			limit $2
		)
		delete from memory_assertion_events
		where ctid in (select ctid from doomed)`,
		`with doomed as (
			select ctid from memory_assertions
			where updated_at < $1
			order by updated_at
			limit $2
		)
		delete from memory_assertions
		where ctid in (select ctid from doomed)`,
		`with doomed as (
			select ctid from memory_assertions
			where expires_at < $1
			order by expires_at
			limit $2
		)
		delete from memory_assertions
		where ctid in (select ctid from doomed)`,
	}
}

func (e *Erasures) markMemorySourcesErased(
	ctx context.Context, conn db, runs []domain.RunID, by domain.UserID, now time.Time,
) (int, error) {
	if len(runs) == 0 {
		return 0, nil
	}
	var count int
	err := conn.QueryRow(ctx, eraseMemorySourcesSQL, memoryRunIDs(runs),
		string(by), now.UTC()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("admin: mark memory source erased: %w", err)
	}
	return count, nil
}

func memoryRunIDs(runs []domain.RunID) []string {
	out := make([]string, 0, len(runs))
	for _, run := range runs {
		out = append(out, string(run))
	}
	return out
}

const eraseMemorySourcesSQL = `
	with updated as (
		update memory_assertions m
		set status = 'source_erased', updated_by = $2, updated_at = $3
		where m.status = 'active'
		  and exists (
		    select 1 from jsonb_array_elements(m.evidence) ev
		    where ev->>'run_id' = any($1::text[])
		  )
		returning m.*
	), recorded as (
		insert into memory_assertion_events (
			assertion_id, action, company_id, area_id, agent_id,
			principal_id, reason, detail, at)
		select assertion_id, 'source_erased', company_id, area_id,
			agent_id, $2, 'source content erased', to_jsonb(updated), $3
		from updated
		returning 1
	)
	select count(*) from recorded`
