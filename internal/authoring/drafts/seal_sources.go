package drafts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

/*
writeSealSources copies where each sealed value came from, so erasure reaches the seal as
it reaches the draft (NT-018 phase 2, delivery P).

With the draft's map current, each part takes its source, and a part the map has no source
for is sealed unknown. With the map stale (an older writer changed the draft without
moving its stamp), no source of a part a conversation can write is trusted: those are
sealed unknown, which any erasure of the draft's threads reaches. The three parts no
proposal can write stay manual, at the latest control entry that could have written them.
Nothing here writes the draft: a seal reads it.
*/
func writeSealSources(ctx context.Context, tx pgx.Tx, sealID string, r row, parts map[string]semanticPart) error {
	names := partNames(parts)
	if r.sourcesCurrent() {
		if _, err := tx.Exec(ctx, `
			insert into agent_seal_sources (seal_id, part, trail_seq)
			select $1::uuid, wanted.name, source.trail_seq
			from unnest($3::text[]) wanted(name)
			left join agent_draft_sources source
			  on source.draft_id = $2::uuid and source.part = wanted.name`,
			sealID, r.draftID, names); err != nil {
			return fmt.Errorf("drafts: seal the sources of %s: %w", r.draftID, err)
		}
		return nil
	}
	for _, name := range names {
		var seq *int64
		if humanPart(name) {
			found, err := latestManualSeq(ctx, tx, r.draftID, name)
			if err != nil {
				return err
			}
			seq = &found
		}
		if _, err := tx.Exec(ctx, `
			insert into agent_seal_sources (seal_id, part, trail_seq) values ($1::uuid, $2, $3)`,
			sealID, name, seq); err != nil {
			return fmt.Errorf("drafts: seal the source of %s: %w", name, err)
		}
	}
	return nil
}

// latestManualSeq is the latest control entry that could have written a human-only part.
func latestManualSeq(ctx context.Context, tx pgx.Tx, draftID, name string) (int64, error) {
	actions := []string{"set_standing_approvals", "create"}
	if name == partName || name == partScope {
		actions = []string{"set_identity", "create"}
	}
	var seq int64
	if err := tx.QueryRow(ctx, `
		select seq from agent_draft_trail
		where draft_id = $1::uuid and origin = 'control' and action = any($2::text[])
		order by seq desc limit 1`, draftID, actions).Scan(&seq); err != nil {
		return 0, fmt.Errorf("drafts: find the manual source of %s: %w", name, err)
	}
	return seq, nil
}
