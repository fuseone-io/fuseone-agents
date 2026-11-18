package drafts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
)

// RepairProvenance seals legacy/stale source maps and repeats erasure for
// threads erased before provenance existed. One broken draft does not prevent
// later drafts from being repaired in the same pass.
func (s *Store) RepairProvenance(ctx context.Context) (int, error) {
	agents, err := s.provenanceRepairCandidates(ctx)
	if err != nil {
		return 0, err
	}
	repaired := 0
	var failures []error
	for _, agent := range agents {
		didRepair, err := s.repairProvenanceAgent(ctx, agent)
		if err != nil {
			failures = append(failures, fmt.Errorf("repair %s: %w", agent, err))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		if didRepair {
			repaired++
		}
	}
	return repaired, errors.Join(failures...)
}

func (s *Store) provenanceRepairCandidates(ctx context.Context) ([]domain.AgentID, error) {
	rows, err := s.pool.Query(ctx, `
		select distinct d.agent_id
		from agent_drafts d
		where d.sources_revision <> d.revision
		   or d.sources_seq <> coalesce((
			select max(t.seq) from agent_draft_trail t where t.draft_id = d.draft_id
		   ), 0)
		   or exists (
			select 1 from agent_draft_threads thread
			where thread.draft_id = d.draft_id and not thread.erasure_reconciled
			  and (
				exists (select 1 from agent_draft_turns turn
				        where turn.thread_id = thread.thread_id and turn.erased)
				or exists (select 1 from run_content content
				           where content.owner_kind = 'authoring'
				             and content.run_id = thread.thread_id::text
				             and content.erased_reason = 'thread erased')
			  )
		   )
		order by d.agent_id`)
	if err != nil {
		return nil, fmt.Errorf("drafts: find provenance repairs: %w", err)
	}
	raw, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	agents := make([]domain.AgentID, len(raw))
	for i := range raw {
		agents[i] = domain.AgentID(raw[i])
	}
	return agents, nil
}

func (s *Store) repairProvenanceAgent(ctx context.Context, agent domain.AgentID) (bool, error) {
	repaired := false
	err := s.fence.Exclusive(ctx, []domain.AgentID{agent}, func(ctx context.Context) error {
		return s.within(ctx, func(tx pgx.Tx) error {
			var err error
			repaired, err = s.repairProvenanceAgentTx(ctx, tx, agent)
			return err
		})
	})
	return repaired, err
}

func (s *Store) repairProvenanceAgentTx(
	ctx context.Context, tx pgx.Tx, agent domain.AgentID,
) (bool, error) {
	r, found, err := readRow(ctx, tx, agent, true)
	if err != nil || !found {
		return false, err
	}
	repaired, err := ensureDraftSources(ctx, tx, &r)
	if err != nil {
		return false, err
	}
	threads, err := erasedThreads(ctx, tx, r.draftID, agent)
	if err != nil {
		return false, err
	}
	if len(threads) > 0 {
		repaired = true
	}
	err = s.eraseThreadsTx(ctx, tx, threadErasure{
		agent: agent, draft: r, threads: threads, reason: "thread erasure repair",
	})
	return repaired, err
}

func ensureDraftSources(ctx context.Context, tx pgx.Tx, r *row) (bool, error) {
	if r.sourcesCurrent() {
		return false, nil
	}
	d, err := draft.Decode(r.stored)
	if err != nil {
		return false, err
	}
	return true, ensureSourceMap(ctx, tx, r, semanticParts(d))
}

func erasedThreads(
	ctx context.Context, tx pgx.Tx, draftID string, agent domain.AgentID,
) ([]string, error) {
	rows, err := tx.Query(ctx, `
		select thread.thread_id::text
		from agent_draft_threads thread
		where thread.draft_id = $1::uuid and not thread.erasure_reconciled
		  and (
			exists (select 1 from agent_draft_turns turn
			        where turn.thread_id = thread.thread_id and turn.erased)
			or exists (select 1 from run_content content
			           where content.owner_kind = 'authoring'
			             and content.run_id = thread.thread_id::text
			             and content.erased_reason = 'thread erased')
		  )
		order by thread.thread_id::text`, draftID)
	if err != nil {
		return nil, fmt.Errorf("drafts: find erased threads of %s: %w", agent, err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}
