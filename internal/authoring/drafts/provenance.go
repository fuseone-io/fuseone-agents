package drafts

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
)

const (
	partName              = draft.SourcePartName
	partScope             = draft.SourcePartScope
	partStandingApprovals = draft.SourcePartStandingApprovals
)

type semanticPart = draft.SourcePart

func semanticParts(d draft.Draft) map[string]semanticPart { return draft.SourceParts(d) }

func partNames(parts map[string]semanticPart) []string {
	out := make([]string, 0, len(parts))
	for name := range parts {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func changedPartNames(before, after map[string]semanticPart) []string {
	all := make(map[string]struct{}, len(before)+len(after))
	for name := range before {
		all[name] = struct{}{}
	}
	for name := range after {
		all[name] = struct{}{}
	}
	out := make([]string, 0, len(all))
	for name := range all {
		if before[name] != after[name] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func humanPart(name string) bool {
	return draft.HumanSourcePart(name)
}

func conversationPart(name string) bool { return !humanPart(name) }

type trailProvenance struct {
	draftID, proposal string
	seq               int64
	parts             []string
}

func writeTrailProvenance(ctx context.Context, tx pgx.Tx, in trailProvenance) error {
	if _, err := tx.Exec(ctx, `
		insert into agent_draft_trail_provenance (draft_id, trail_seq, proposal_id, parts)
		values ($1::uuid, $2, nullif($3, '')::uuid, $4::text[])`,
		in.draftID, in.seq, in.proposal, in.parts); err != nil {
		return fmt.Errorf("drafts: keep provenance of %s #%d: %w", in.draftID, in.seq, err)
	}
	return nil
}

type sourcePosition struct {
	draftID, part string
	seq           int64
}

func setSource(ctx context.Context, tx pgx.Tx, in sourcePosition) error {
	if _, err := tx.Exec(ctx, `
		insert into agent_draft_sources (draft_id, part, trail_seq)
		values ($1::uuid, $2, $3)
		on conflict (draft_id, part) do update set trail_seq = excluded.trail_seq`,
		in.draftID, in.part, in.seq); err != nil {
		return fmt.Errorf("drafts: source %s of %s: %w", in.part, in.draftID, err)
	}
	return nil
}

func deleteSource(ctx context.Context, tx pgx.Tx, draftID, part string) error {
	if _, err := tx.Exec(ctx,
		`delete from agent_draft_sources where draft_id = $1::uuid and part = $2`, draftID, part); err != nil {
		return fmt.Errorf("drafts: clear source %s of %s: %w", part, draftID, err)
	}
	return nil
}

type sourceStamp struct {
	draftID  string
	revision domain.Revision
	seq      int64
}

func stampSources(ctx context.Context, tx pgx.Tx, in sourceStamp) error {
	if _, err := tx.Exec(ctx, `
		update agent_drafts set sources_revision = $2, sources_seq = $3
		where draft_id = $1::uuid`, in.draftID, string(in.revision), in.seq); err != nil {
		return fmt.Errorf("drafts: stamp sources of %s: %w", in.draftID, err)
	}
	return nil
}

// ensureSourceMap applies the one legacy rule before any writer or eraser
// judges provenance: with no accepted proposal every current value is manual;
// otherwise conversation-capable values remain unknown.
func ensureSourceMap(
	ctx context.Context, tx pgx.Tx, r *row, parts map[string]semanticPart,
) error {
	if r.sourcesCurrent() {
		return nil
	}
	if err := backfillSources(ctx, tx, *r, parts); err != nil {
		return err
	}
	r.sources, r.sourcesSeq = r.revision, r.lastTrailSeq
	return nil
}

// seedHumanSources keeps the three fields proposals cannot write manual even
// when an old replica made the rest of the map stale.
func seedHumanSources(
	ctx context.Context, tx pgx.Tx, draftID string, parts map[string]semanticPart,
) error {
	bySeq := make(map[int64][]string)
	for _, name := range []string{partName, partScope, partStandingApprovals} {
		if _, present := parts[name]; !present {
			continue
		}
		var seq int64
		actions := []string{"create"}
		if name == partName || name == partScope {
			actions = append([]string{"set_identity"}, actions...)
		} else {
			actions = append([]string{"set_standing_approvals"}, actions...)
		}
		if err := tx.QueryRow(ctx, `
			select seq from agent_draft_trail
			where draft_id = $1::uuid and origin = 'control' and action = any($2::text[])
			order by seq desc limit 1`, draftID, actions).Scan(&seq); err != nil {
			return fmt.Errorf("drafts: find manual source of %s: %w", name, err)
		}
		bySeq[seq] = append(bySeq[seq], name)
	}
	for seq, names := range bySeq {
		sort.Strings(names)
		if _, err := tx.Exec(ctx, `
			insert into agent_draft_trail_provenance (draft_id, trail_seq, proposal_id, parts)
			values ($1::uuid, $2, null, $3::text[])
			on conflict (draft_id, trail_seq) do nothing`, draftID, seq, names); err != nil {
			return fmt.Errorf("drafts: seed manual provenance of %s #%d: %w", draftID, seq, err)
		}
		for _, name := range names {
			if err := setSource(ctx, tx, sourcePosition{draftID, name, seq}); err != nil {
				return err
			}
		}
	}
	return nil
}

// backfillSources seals a draft written before provenance existed, or made
// stale by an old replica. With no accepted proposal in its trail every
// current value is known to be manual. Once a proposal has been accepted,
// conversation-capable values are conservatively left unknown; the three
// values proposals cannot write remain manual.
func backfillSources(
	ctx context.Context, tx pgx.Tx, r row, parts map[string]semanticPart,
) error {
	if _, err := tx.Exec(ctx, `delete from agent_draft_sources where draft_id = $1::uuid`, r.draftID); err != nil {
		return fmt.Errorf("drafts: reset stale sources of %s: %w", r.draftID, err)
	}
	var hasProposal bool
	if err := tx.QueryRow(ctx, `
		select exists (
			select 1 from agent_draft_trail
			where draft_id = $1::uuid and origin = 'proposal'
		)`, r.draftID).Scan(&hasProposal); err != nil {
		return fmt.Errorf("drafts: inspect legacy trail of %s: %w", r.draftID, err)
	}
	if hasProposal {
		if err := seedHumanSources(ctx, tx, r.draftID, parts); err != nil {
			return err
		}
	} else {
		names := partNames(parts)
		if _, err := tx.Exec(ctx, `
			insert into agent_draft_trail_provenance (draft_id, trail_seq, proposal_id, parts)
			values ($1::uuid, $2, null, $3::text[])
			on conflict (draft_id, trail_seq) do nothing`, r.draftID, r.lastTrailSeq, names); err != nil {
			return fmt.Errorf("drafts: seed legacy provenance of %s: %w", r.draftID, err)
		}
		for _, name := range names {
			if err := setSource(ctx, tx, sourcePosition{r.draftID, name, r.lastTrailSeq}); err != nil {
				return err
			}
		}
	}
	return stampSources(ctx, tx, sourceStamp{r.draftID, r.revision, r.lastTrailSeq})
}
