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
		deleteMemoryRowsBy("memory_assertion_events", "at", "at < $1"),
		deleteMemoryRowsBy("memory_assertions", "updated_at", "updated_at < $1"),
		deleteMemoryRowsBy("memory_assertions", "expires_at", "expires_at < $1"),
		deleteMemoryRowsBy("memory_suggestions", "updated_at", "updated_at < $1"),
		deleteMemoryRowsBy("memory_suggestions", "expires_at", "expires_at < $1"),
	}
}

func deleteMemoryRowsBy(table, orderedBy, predicate string) string {
	return fmt.Sprintf(`with doomed as (
			select ctid from %s
			where %s
			order by %s
			limit $2
		)
		delete from %s
		where ctid in (select ctid from doomed)`, table, predicate, orderedBy, table)
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
	with updated_assertions as (
		update memory_assertions m
		set status = 'source_erased', updated_by = $2, updated_at = $3
		where m.status = 'active'
		  and exists (
		    select 1 from jsonb_array_elements(m.evidence) ev
		    where ev->>'run_id' = any($1::text[])
		  )
		returning m.*
	), recorded_assertions as (
		insert into memory_assertion_events (
			assertion_id, action, company_id, area_id, agent_id,
			principal_id, reason, detail, at)
		select assertion_id, 'source_erased', company_id, area_id,
			agent_id, $2, 'source content erased', to_jsonb(updated_assertions), $3
		from updated_assertions
		returning 1
	), updated_suggestions as (
		update memory_suggestions m
		set status = 'source_erased', updated_by = $2, updated_at = $3
		where m.status = 'pending'
		  and exists (
		    select 1 from jsonb_array_elements(m.evidence) ev
		    where ev->>'run_id' = any($1::text[])
		  )
		returning m.*
	), recorded_suggestions as (
		insert into memory_assertion_events (
			assertion_id, action, company_id, area_id, agent_id,
			principal_id, reason, detail, at)
		select assertion_id, 'source_erased', company_id, area_id,
			agent_id, $2, 'suggestion source content erased', to_jsonb(updated_suggestions), $3
		from updated_suggestions
		returning 1
	)
	select (select count(*) from recorded_assertions) +
	       (select count(*) from recorded_suggestions)`
