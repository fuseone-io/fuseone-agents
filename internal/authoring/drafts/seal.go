package drafts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

// Sealing asks to seal the agent's draft at the revision the caller read.
type Sealing struct {
	Agent  domain.AgentID
	Expect domain.IfMatch
	Caller Caller
}

// Sealed is what a seal wrote, or the published version it found instead.
type Sealed struct {
	// SealID is empty when the draft's bytes are an already-published version:
	// nothing is sealed, and that version is the answer.
	SealID    string
	Version   domain.VersionID
	Scope     domain.Scope
	Published bool
	// Judged is the event cycles the version would close, for the author to see.
	Judged spec.Published
}

/*
Seal keeps the draft's definition as a candidate (NT-018 phase 2, release 2b).

The bytes are the ones a publication of this draft would write, rendered here from the
draft at the revision the caller read, so the version id is the one a publication would
compute. Nothing in agent_specs changes: a candidate is invisible to every reader of the
current version.

The order is the plan's. The content fence, shared, comes first and stays until the commit,
so an erasure of the draft's thread either waits for the seal or is waited for and leaves
its tombstone where the seal reads. The event graph, shared, keeps the claim race-free
against a publication while seals do not wait for each other. The draft row is read without
a lock: taking it before the claim would invert the order the workbench's own publication
takes them in.
*/
func (s *Store) Seal(ctx context.Context, in Sealing) (Sealed, error) {
	var out Sealed
	err := s.fence.Hold(ctx, in.Agent, func(ctx context.Context) error {
		return s.within(ctx, func(tx pgx.Tx) error {
			var err error
			out, err = s.sealTx(ctx, tx, in)
			return err
		})
	})
	return out, err
}

func (s *Store) sealTx(ctx context.Context, tx pgx.Tx, in Sealing) (Sealed, error) {
	if err := spec.ShareEventGraph(ctx, tx); err != nil {
		return Sealed{}, err
	}
	r, d, err := sealableRow(ctx, tx, in)
	if err != nil {
		return Sealed{}, err
	}
	rendered, version, err := sealedDefinition(d)
	if err != nil {
		return Sealed{}, err
	}
	out := Sealed{Version: version.Version, Scope: r.scope}
	if err := spec.ClaimForSeal(ctx, tx, version); err != nil {
		return Sealed{}, err
	}
	if out.Published, err = spec.PublishedIn(ctx, tx, in.Agent, version.Version); err != nil || out.Published {
		return out, err
	}
	if out.SealID, err = s.writeSeal(ctx, tx, sealWrite{in, r, version, rendered}); err != nil {
		return Sealed{}, err
	}
	if err := writeSealSources(ctx, tx, out.SealID, r, semanticParts(d)); err != nil {
		return Sealed{}, err
	}
	out.Judged, err = spec.JudgeIn(ctx, tx, version)
	return out, err
}

// sealableRow is the draft the caller may seal, at the revision they read.
func sealableRow(ctx context.Context, tx pgx.Tx, in Sealing) (row, draft.Draft, error) {
	current, published, err := spec.CurrentIn(ctx, tx, in.Agent)
	if err != nil {
		return row{}, draft.Draft{}, err
	}
	r, found, err := readRow(ctx, tx, in.Agent, false)
	if err != nil {
		return row{}, draft.Draft{}, err
	}
	if !found || !reachable(in.Caller, r.scope, current, published) {
		return row{}, draft.Draft{}, ErrNotFound
	}
	if !in.Expect.Allows(true, r.revision) {
		return row{}, draft.Draft{}, &domain.StaleError{Current: r.revision}
	}
	d, err := draft.Decode(r.stored)
	return r, d, err
}

// sealedDefinition is the draft as a publication would write it: rendered, and parsed
// back so the parser's validation and digest are the publication's.
func sealedDefinition(d draft.Draft) ([]byte, spec.Spec, error) {
	rendered, err := spec.Render(d.Definition())
	if err != nil {
		return nil, spec.Spec{}, fmt.Errorf("drafts: render the seal: %w", err)
	}
	parsed, err := spec.Parse("seal", rendered)
	if err != nil {
		return nil, spec.Spec{}, fmt.Errorf("drafts: the draft is not a definition: %w", err)
	}
	return rendered, parsed, nil
}

type sealWrite struct {
	in       Sealing
	draft    row
	version  spec.Spec
	rendered []byte
}

// writeSeal keeps the bytes exactly, under the seal's own id, and writes its row.
func (s *Store) writeSeal(ctx context.Context, tx pgx.Tx, w sealWrite) (string, error) {
	var id string
	if err := tx.QueryRow(ctx, `select gen_random_uuid()::text`).Scan(&id); err != nil {
		return "", fmt.Errorf("drafts: name the seal: %w", err)
	}
	ref, err := s.content.PutExactForTx(ctx, tx, "authoring", id, 0, w.rendered)
	if err != nil {
		return "", fmt.Errorf("drafts: keep the sealed bytes: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		insert into agent_seals (seal_id, agent_id, version_id, company_id, area_id, content_ref,
		  draft_id, draft_revision, sealed_by)
		values ($1::uuid, $2, $3, $4, $5, $6, $7::uuid, $8, $9)`,
		id, string(w.in.Agent), string(w.version.Version), string(w.draft.scope.Company),
		string(w.draft.scope.Area), ref, w.draft.draftID, string(w.draft.revision),
		string(w.in.Caller.By)); err != nil {
		return "", fmt.Errorf("drafts: write the seal of %s: %w", w.in.Agent, err)
	}
	return id, nil
}
