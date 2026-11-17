package drafts

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

/*
Create opens an agent's draft, if it has none (If-None-Match: *).

With an identity, for an agent that does not exist yet: the human step at its
birth, judged on the scope it chooses. Without one, for a published agent:
the draft opens from its current version, untouched, judged on where it
lives. A draft that exists already answers with its tag when the caller may
reach it, and as no draft at all otherwise; so does a published agent out of
the caller's reach.
*/
func (s *Store) Create(ctx context.Context, agent domain.AgentID, identity *Identity, c Caller) (Opened, error) {
	var out Opened
	err := s.within(ctx, func(tx pgx.Tx) error {
		current, published, err := spec.CurrentIn(ctx, tx, agent)
		if err != nil {
			return err
		}
		if r, found, err := readRow(ctx, tx, agent, true); err != nil || found {
			return firstOrErr(err, existing(c, r, current, published))
		}
		d, base, err := birth(agent, identity, current, published, c)
		if err != nil {
			return err
		}
		out, err = s.insert(ctx, tx, insertion{agent: agent, d: d, base: base, by: c.By},
			func(r row) error { return existing(c, r, current, published) })
		return err
	})
	return out, err
}

func firstOrErr(err, otherwise error) error {
	if err != nil {
		return err
	}
	return otherwise
}

// existing is the answer to opening a draft that is already there.
func existing(c Caller, r row, current spec.Spec, published bool) error {
	if !reachable(c, r.scope, current, published) {
		return ErrNotFound
	}
	return &domain.StaleError{Current: r.revision}
}

// birth is the draft a creation starts from, and the version it opens.
func birth(
	agent domain.AgentID, identity *Identity, current spec.Spec, published bool, c Caller,
) (draft.Draft, domain.VersionID, error) {
	if identity == nil {
		if !published || !c.Allow(publishedScope(current)) {
			return draft.Draft{}, "", ErrNotFound
		}
		return draft.FromSpec(current), current.Version, nil
	}
	switch {
	case published && c.Allow(publishedScope(current)):
		return draft.Draft{}, "", ErrAgentExists
	case published:
		return draft.Draft{}, "", ErrNotFound
	}
	// What publication would refuse as a scope is refused here, as a
	// request error, before the table's constraint could be the one to.
	if err := draft.ValidScope(identity.Scope); err != nil {
		return draft.Draft{}, "", fmt.Errorf("%w: %v", draft.ErrInvalidOperation, err)
	}
	if !c.Allow(identity.Scope) {
		return draft.Draft{}, "", &ForbiddenError{Scope: identity.Scope}
	}
	return draft.New(agent, identity.Name, identity.Scope), "", nil
}

type insertion struct {
	agent domain.AgentID
	d     draft.Draft
	base  domain.VersionID
	by    domain.UserID
}

// insert writes a new draft. Another creation of the same agent racing this
// one makes the insert wait for it and then do nothing; what it committed is
// answered through lost.
func (s *Store) insert(ctx context.Context, tx pgx.Tx, in insertion, lost func(row) error) (Opened, error) {
	st, err := draft.Encode(in.d)
	if err != nil {
		return Opened{}, err
	}
	var draftID string
	if err := tx.QueryRow(ctx, `select gen_random_uuid()::text`).Scan(&draftID); err != nil {
		return Opened{}, fmt.Errorf("drafts: name the draft: %w", err)
	}
	revision := revisionOf(draftID, st)
	scope := domain.Scope{Company: in.d.Spec.Company, Area: in.d.Spec.Area}
	tag, err := tx.Exec(ctx, `
		insert into agent_drafts (agent_id, draft_id, company_id, area_id, definition, blocks,
			base_version, revision, created_by, created_at, updated_by, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), $9, now())
		on conflict (agent_id) do nothing`,
		string(in.agent), draftID, string(scope.Company), string(scope.Area), st.Definition, st.Blocks,
		string(in.base), string(revision), string(in.by))
	if err != nil {
		return Opened{}, fmt.Errorf("drafts: create %s: %w", in.agent, err)
	}
	if tag.RowsAffected() == 0 {
		r, found, err := readRow(ctx, tx, in.agent, true)
		if err != nil || !found {
			return Opened{}, firstOrErr(err, fmt.Errorf("drafts: %s: lost a creation to one that is gone", in.agent))
		}
		return Opened{}, lost(r)
	}
	if err := s.appendTrail(ctx, tx, trailRow{
		draftID: draftID, agent: in.agent, action: "create", body: creationBody(in),
		origin: draft.FromControl, revision: revision, by: in.by,
	}); err != nil {
		return Opened{}, err
	}
	return opened(in.agent, row{draftID: draftID, scope: scope, stored: st, base: string(in.base), revision: revision}, in.d)
}

// Application is one operation on a draft.
type Application struct {
	Agent     domain.AgentID
	Op        draft.Operation
	Origin    draft.Origin
	Expect    domain.IfMatch
	Caller    Caller
	Validator draft.Validator
	Now       time.Time
	// kept is the accepted proposal's body, which the trail names.
	kept *keptBody
}

/*
Apply applies one operation to a draft, under its row's lock.

In order: If-Match is required; who may reach the draft, on its scope and on
where the agent lives now (404); the scope the operation moves it to, derived
with no check that depends on it (403); the tag (412); then the operation,
validated over the canonical form.
Nothing is written unless all of it holds.
*/
func (s *Store) Apply(ctx context.Context, a Application) (Opened, error) {
	if !a.Expect.Present() {
		return Opened{}, ErrPreconditionRequired
	}
	var out Opened
	err := s.within(ctx, func(tx pgx.Tx) error {
		r, err := s.lockReachable(ctx, tx, a.Agent, a.Caller)
		if err != nil {
			return err
		}
		d, err := draft.Decode(r.stored)
		if err != nil {
			return err
		}
		// The scope the operation leaves, derived without a check that
		// depends on it and judged before them and before the tag: a caller
		// moving a draft where it may not publish learns nothing there.
		// Whatever operation and form: judged on the result, not the type.
		if err := destinationAllowed(d, r.scope, a); err != nil {
			return err
		}
		if !a.Expect.Allows(true, r.revision) {
			return &domain.StaleError{Current: r.revision}
		}
		// Validate applies the same operation to the same copy, so the
		// draft it returns is in the scope destinationAllowed judged.
		applied, err := a.Validator.Validate(ctx, d, a.Op, a.Origin, a.Now)
		if err != nil {
			return err
		}
		out, err = s.update(ctx, tx, r, a, applied)
		return err
	})
	return out, err
}

// destinationAllowed refuses an operation that would move the draft to a
// scope the caller may not publish into. An operation that cannot apply moves
// nothing; Validate reports why, after the tag.
func destinationAllowed(d draft.Draft, from domain.Scope, a Application) error {
	preview, err := draft.Preview(d, a.Op)
	if err != nil {
		return nil
	}
	if moved := scopeOf(preview); moved != from && !a.Caller.Allow(moved) {
		return &ForbiddenError{Scope: moved}
	}
	return nil
}

// lockReachable locks the draft's row and judges contract 5 on it.
func (s *Store) lockReachable(ctx context.Context, tx pgx.Tx, agent domain.AgentID, c Caller) (row, error) {
	current, published, err := spec.CurrentIn(ctx, tx, agent)
	if err != nil {
		return row{}, err
	}
	r, found, err := readRow(ctx, tx, agent, true)
	if err != nil {
		return row{}, err
	}
	if !found || !reachable(c, r.scope, current, published) {
		return row{}, ErrNotFound
	}
	return r, nil
}

func (s *Store) update(ctx context.Context, tx pgx.Tx, r row, a Application, applied draft.Applied) (Opened, error) {
	st, err := draft.Encode(applied.Draft)
	if err != nil {
		return Opened{}, err
	}
	revision := revisionOf(r.draftID, st)
	scope := domain.Scope{Company: applied.Draft.Spec.Company, Area: applied.Draft.Spec.Area}
	if _, err := tx.Exec(ctx, `
		update agent_drafts set company_id = $2, area_id = $3, definition = $4, blocks = $5,
			revision = $6, updated_by = $7, updated_at = now()
		where agent_id = $1`,
		string(a.Agent), string(scope.Company), string(scope.Area), st.Definition, st.Blocks,
		string(revision), string(a.Caller.By)); err != nil {
		return Opened{}, fmt.Errorf("drafts: write %s: %w", a.Agent, err)
	}
	if err := s.appendTrail(ctx, tx, trailRow{
		draftID: r.draftID, agent: a.Agent, action: a.Op.Kind(), body: draft.Body(a.Op), kept: a.kept,
		origin: a.Origin, revision: revision, by: a.Caller.By,
	}); err != nil {
		return Opened{}, err
	}
	return Opened{
		Agent: a.Agent, DraftID: r.draftID, Draft: applied.Draft, Missing: applied.Missing,
		Revision: revision, BaseVersion: domain.VersionID(r.base),
	}, nil
}

// Discard removes a draft, against the tag the caller read. Its trail stays.
func (s *Store) Discard(ctx context.Context, agent domain.AgentID, expect domain.IfMatch, c Caller) error {
	if !expect.Present() {
		return ErrPreconditionRequired
	}
	return s.within(ctx, func(tx pgx.Tx) error {
		r, err := s.lockReachable(ctx, tx, agent, c)
		if err != nil {
			return err
		}
		if !expect.Allows(true, r.revision) {
			return &domain.StaleError{Current: r.revision}
		}
		if _, err := tx.Exec(ctx, `delete from agent_drafts where agent_id = $1`, string(agent)); err != nil {
			return fmt.Errorf("drafts: discard %s: %w", agent, err)
		}
		return s.appendTrail(ctx, tx, trailRow{
			draftID: r.draftID, agent: agent, action: "discard",
			origin: draft.FromControl, revision: r.revision, by: c.By,
		})
	})
}
