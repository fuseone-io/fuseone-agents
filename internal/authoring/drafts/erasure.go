package drafts

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
)

type threadErasure struct {
	agent   domain.AgentID
	draft   row
	threads []string
	by      domain.UserID
	reason  string
}

// eraseThreadsTx erases the conversation and every current draft value that
// depends on it. The caller holds the corpus fence, event-graph lock and draft
// row lock, in that order.
func (s *Store) eraseThreadsTx(ctx context.Context, tx pgx.Tx, in threadErasure) error {
	if len(in.threads) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		update agent_draft_turns set erased = true
		where thread_id = any($1::uuid[])`, in.threads); err != nil {
		return fmt.Errorf("drafts: fence erased turns: %w", err)
	}
	if _, err := s.content.EraseOwnersTx(ctx, tx, "authoring", in.threads, in.reason); err != nil {
		return err
	}
	return s.eraseThreadDraftValues(ctx, tx, in)
}

func (s *Store) eraseThreadDraftValues(ctx context.Context, tx pgx.Tx, in threadErasure) error {
	d, err := draft.Decode(in.draft.stored)
	if err != nil {
		return err
	}
	parts := semanticParts(d)
	if err := ensureSourceMap(ctx, tx, &in.draft, parts); err != nil {
		return err
	}
	names, err := affectedConversationParts(ctx, tx, sourceLookup{
		draftID: in.draft.draftID, current: parts, references: in.threads,
	})
	if err != nil {
		return err
	}
	if len(names) > 0 {
		if _, err := s.eraseDraftPartsTx(ctx, tx, draftPartErasure{
			in.agent, in.draft, d, names, in.by,
		}); err != nil {
			return err
		}
	}
	return markThreadsReconciled(ctx, tx, in.threads)
}

type draftPartErasure struct {
	agent domain.AgentID
	row   row
	draft draft.Draft
	names []string
	by    domain.UserID
}

func (s *Store) eraseDraftPartsTx(ctx context.Context, tx pgx.Tx, in draftPartErasure) (int, error) {
	if in.by == "" {
		in.by = "system:provenance-repair"
	}
	draft.EraseSourceParts(&in.draft, in.names)
	stored, err := draft.Encode(in.draft)
	if err != nil {
		return 0, err
	}
	revision := revisionOf(in.row.draftID, stored)
	if err := writeErasedDraft(ctx, tx, erasedDraftWrite{in, stored, revision}); err != nil {
		return 0, err
	}
	seq, err := s.appendTrail(ctx, tx, trailRow{
		draftID: in.row.draftID, agent: in.agent, action: "erase", origin: draft.FromControl,
		revision: revision, by: in.by,
	})
	if err != nil {
		return 0, err
	}
	if err := writeTrailProvenance(ctx, tx, trailProvenance{
		draftID: in.row.draftID, seq: seq, parts: in.names,
	}); err != nil {
		return 0, err
	}
	for _, name := range in.names {
		if err := deleteSource(ctx, tx, in.row.draftID, name); err != nil {
			return 0, err
		}
	}
	if err := stampSources(ctx, tx, sourceStamp{in.row.draftID, revision, seq}); err != nil {
		return 0, err
	}
	return len(in.names), nil
}

type erasedDraftWrite struct {
	erasure  draftPartErasure
	stored   draft.Stored
	revision domain.Revision
}

func writeErasedDraft(ctx context.Context, tx pgx.Tx, in erasedDraftWrite) error {
	e := in.erasure
	if _, err := tx.Exec(ctx, `
		update agent_drafts
		set definition = $2, blocks = $3, revision = $4,
		    updated_by = $5, updated_at = now()
		where draft_id = $1::uuid`, e.row.draftID, in.stored.Definition, in.stored.Blocks,
		string(in.revision), string(e.by)); err != nil {
		return fmt.Errorf("drafts: erase values of %s: %w", e.agent, err)
	}
	return nil
}

func markThreadsReconciled(ctx context.Context, tx pgx.Tx, threads []string) error {
	if _, err := tx.Exec(ctx, `
		update agent_draft_threads set erasure_reconciled = true
		where thread_id = any($1::uuid[])`, threads); err != nil {
		return fmt.Errorf("drafts: mark thread erasure reconciled: %w", err)
	}
	return nil
}

type sourceLookup struct {
	draftID    string
	current    map[string]semanticPart
	references []string
}

// affectedConversationParts finds current values whose latest writer was a
// proposal in one of threads. A current value with no trustworthy source is
// deliberately included: after conversation erasure its origin cannot be
// proved manual.
func affectedConversationParts(ctx context.Context, tx pgx.Tx, in sourceLookup) ([]string, error) {
	rows, err := tx.Query(ctx, `
		select s.part, exists (
			select 1 from agent_draft_trail_provenance p
			join agent_draft_proposals proposal on proposal.proposal_id = p.proposal_id
			where p.draft_id = s.draft_id and p.trail_seq = s.trail_seq
			  and proposal.thread_id = any($2::uuid[])
		)
		from agent_draft_sources s where s.draft_id = $1::uuid`, in.draftID, in.references)
	if err != nil {
		return nil, fmt.Errorf("drafts: read sources before erasure: %w", err)
	}
	defer rows.Close()
	known := make(map[string]bool)
	for rows.Next() {
		var name string
		var erasedThread bool
		if err := rows.Scan(&name, &erasedThread); err != nil {
			return nil, err
		}
		known[name] = erasedThread
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var affected []string
	for name := range in.current {
		if conversationPart(name) && (known[name] || sourceMissing(known, name)) {
			affected = append(affected, name)
		}
	}
	sort.Strings(affected)
	return affected, nil
}

func sourceMissing(known map[string]bool, name string) bool {
	_, present := known[name]
	return !present
}
