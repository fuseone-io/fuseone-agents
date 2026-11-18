package drafts

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

// EraseReferencedRunsTx reaches authoring content cited by a subject-erasure
// request. The caller owns the transaction and holds the corpus fence for the
// agents of runs. Only turns that explicitly cite a run are touched.
func (s *Store) EraseReferencedRunsTx(
	ctx context.Context, tx pgx.Tx, runs []domain.RunID, by domain.UserID,
) (int, error) {
	if len(runs) == 0 {
		return 0, nil
	}
	rawRuns := make([]string, len(runs))
	for i, run := range runs {
		rawRuns[i] = string(run)
	}
	if err := spec.ShareEventGraph(ctx, tx); err != nil {
		return 0, err
	}
	agents, err := agentsReferencingRuns(ctx, tx, rawRuns)
	if err != nil {
		return 0, err
	}
	erased := 0
	for _, agent := range agents {
		n, err := s.eraseAgentRunReferences(ctx, tx, subjectErasure{agent, rawRuns, by})
		if err != nil {
			return 0, err
		}
		erased += n
	}
	return erased, nil
}

type subjectErasure struct {
	agent domain.AgentID
	runs  []string
	by    domain.UserID
}

func agentsReferencingRuns(ctx context.Context, tx pgx.Tx, runs []string) ([]domain.AgentID, error) {
	rows, err := tx.Query(ctx, `
		select distinct agent_id from agent_draft_turns
		where referenced_run = any($1::text[]) order by agent_id`, runs)
	if err != nil {
		return nil, fmt.Errorf("drafts: find turns that cite erased runs: %w", err)
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

func (s *Store) eraseAgentRunReferences(ctx context.Context, tx pgx.Tx, in subjectErasure) (int, error) {
	refs, err := authoringRefsForRuns(ctx, tx, in.agent, in.runs)
	if err != nil {
		return 0, err
	}
	erased, err := s.content.EraseRefsTx(ctx, tx, refs, "subject")
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		update agent_draft_turns set subject_erased = true
		where agent_id = $1 and referenced_run = any($2::text[])`, string(in.agent), in.runs); err != nil {
		return 0, fmt.Errorf("drafts: mark turns for erased runs: %w", err)
	}
	n, err := s.eraseDraftRunReferences(ctx, tx, in)
	return erased + n, err
}

func (s *Store) eraseDraftRunReferences(ctx context.Context, tx pgx.Tx, in subjectErasure) (int, error) {
	r, found, err := readRow(ctx, tx, in.agent, true)
	if err != nil || !found {
		return 0, err
	}
	proposals, err := proposalsForRuns(ctx, tx, r.draftID, in.runs)
	if err != nil {
		return 0, err
	}
	sealed, err := s.eraseSealsReachedTx(ctx, tx, sealReach{
		draftID: r.draftID, proposals: proposals, reason: "subject",
	})
	if err != nil {
		return 0, err
	}
	d, err := draft.Decode(r.stored)
	if err != nil {
		return 0, err
	}
	parts := semanticParts(d)
	if err := ensureSourceMap(ctx, tx, &r, parts); err != nil {
		return 0, err
	}
	names, err := affectedProposalParts(ctx, tx, sourceLookup{r.draftID, parts, proposals})
	if err != nil || len(names) == 0 {
		return sealed, err
	}
	n, err := s.eraseDraftPartsTx(ctx, tx, draftPartErasure{in.agent, r, d, names, in.by})
	return sealed + n, err
}

func proposalsForRuns(ctx context.Context, tx pgx.Tx, draftID string, runs []string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		select p.proposal_id::text
		from agent_draft_proposals p
		join agent_draft_turns turn on turn.turn_id = p.turn_id
		where turn.draft_id = $1::uuid and turn.referenced_run = any($2::text[])
		order by p.proposal_id::text`, draftID, runs)
	if err != nil {
		return nil, fmt.Errorf("drafts: find proposals for erased runs: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func authoringRefsForRuns(
	ctx context.Context, tx pgx.Tx, agent domain.AgentID, runs []string,
) ([]string, error) {
	rows, err := tx.Query(ctx, `
		select ref from (
			select message_ref as ref from agent_draft_turns
			where agent_id = $1 and referenced_run = any($2::text[])
			union
			select nullif(reply_ref, '') from agent_draft_turns
			where agent_id = $1 and referenced_run = any($2::text[])
			union
			select nullif(p.body_ref, '') from agent_draft_proposals p
			join agent_draft_turns turn on turn.turn_id = p.turn_id
			where turn.agent_id = $1 and turn.referenced_run = any($2::text[])
			union
			select nullif(p.reason_ref, '') from agent_draft_proposals p
			join agent_draft_turns turn on turn.turn_id = p.turn_id
			where turn.agent_id = $1 and turn.referenced_run = any($2::text[])
		) refs where ref is not null`, string(agent), runs)
	if err != nil {
		return nil, fmt.Errorf("drafts: find authoring content for erased runs: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func affectedProposalParts(ctx context.Context, tx pgx.Tx, in sourceLookup) ([]string, error) {
	rows, err := tx.Query(ctx, `
		select s.part from agent_draft_sources s
		join agent_draft_trail_provenance p
		  on p.draft_id = s.draft_id and p.trail_seq = s.trail_seq
		where s.draft_id = $1::uuid and p.proposal_id = any($2::uuid[])`, in.draftID, in.references)
	if err != nil {
		return nil, fmt.Errorf("drafts: read proposal sources before erasure: %w", err)
	}
	sourced, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	affected := stringSet(sourced)
	known, err := knownSourceParts(ctx, tx, in.draftID)
	if err != nil {
		return nil, err
	}
	var out []string
	for name := range in.current {
		if conversationPart(name) && (affected[name] || !known[name]) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

func knownSourceParts(ctx context.Context, tx pgx.Tx, draftID string) (map[string]bool, error) {
	rows, err := tx.Query(ctx,
		`select part from agent_draft_sources where draft_id = $1::uuid`, draftID)
	if err != nil {
		return nil, err
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	return stringSet(names), nil
}

func stringSet(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name] = true
	}
	return out
}
