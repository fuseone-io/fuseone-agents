package drafts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
)

func (s *Store) update(ctx context.Context, tx pgx.Tx, r row, a Application, applied draft.Applied) (Opened, error) {
	before, err := draft.Decode(r.stored)
	if err != nil {
		return Opened{}, err
	}
	beforeParts := semanticParts(before)
	if err := ensureSourceMap(ctx, tx, &r, beforeParts); err != nil {
		return Opened{}, err
	}
	stored, err := draft.Encode(applied.Draft)
	if err != nil {
		return Opened{}, err
	}
	revision := revisionOf(r.draftID, stored)
	if err := writeDraft(ctx, tx, draftWrite{
		application: a, stored: stored, revision: revision,
		scope: domain.Scope{Company: applied.Draft.Spec.Company, Area: applied.Draft.Spec.Area},
	}); err != nil {
		return Opened{}, err
	}
	seq, err := s.appendTrail(ctx, tx, trailRow{
		draftID: r.draftID, agent: a.Agent, action: a.Op.Kind(), body: draft.Body(a.Op), kept: a.kept,
		origin: a.Origin, revision: revision, by: a.Caller.By,
	})
	if err != nil {
		return Opened{}, err
	}
	afterParts := semanticParts(applied.Draft)
	if err := updateSources(ctx, tx, sourceUpdate{
		draftID: r.draftID, seq: seq, proposal: a.proposal, revision: revision,
		before: beforeParts, after: afterParts,
	}); err != nil {
		return Opened{}, err
	}
	return Opened{
		Agent: a.Agent, DraftID: r.draftID, Draft: applied.Draft, Missing: applied.Missing,
		Revision: revision, BaseVersion: domain.VersionID(r.base),
	}, nil
}

type draftWrite struct {
	application Application
	stored      draft.Stored
	revision    domain.Revision
	scope       domain.Scope
}

func writeDraft(ctx context.Context, tx pgx.Tx, in draftWrite) error {
	a := in.application
	if _, err := tx.Exec(ctx, `
		update agent_drafts set company_id = $2, area_id = $3, definition = $4, blocks = $5,
			revision = $6, updated_by = $7, updated_at = now()
		where agent_id = $1`,
		string(a.Agent), string(in.scope.Company), string(in.scope.Area), in.stored.Definition, in.stored.Blocks,
		string(in.revision), string(a.Caller.By)); err != nil {
		return fmt.Errorf("drafts: write %s: %w", a.Agent, err)
	}
	return nil
}

type sourceUpdate struct {
	draftID, proposal string
	seq               int64
	revision          domain.Revision
	before, after     map[string]semanticPart
}

func updateSources(ctx context.Context, tx pgx.Tx, in sourceUpdate) error {
	changed := changedPartNames(in.before, in.after)
	if err := writeTrailProvenance(ctx, tx, trailProvenance{
		draftID: in.draftID, seq: in.seq, proposal: in.proposal, parts: changed,
	}); err != nil {
		return err
	}
	for _, name := range changed {
		if _, present := in.after[name]; present {
			if err := setSource(ctx, tx, sourcePosition{in.draftID, name, in.seq}); err != nil {
				return err
			}
		} else if err := deleteSource(ctx, tx, in.draftID, name); err != nil {
			return err
		}
	}
	return stampSources(ctx, tx, sourceStamp{in.draftID, in.revision, in.seq})
}
