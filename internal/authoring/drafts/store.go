// Package drafts stores the workbench's drafts (NT-018 1b, part 3).
//
// One draft per agent, in agent_drafts, changed only by an operation applied
// under the row's lock against the ETag the caller read, each leaving a row in
// the append-only agent_draft_trail of ids and digests.
//
// Every write takes the event graph's lock shared, then the draft's row: the
// order a publication takes the graph exclusively and then the agent's row, so
// the workbench's publication can take all three without a deadlock. Under
// it, who may reach the draft is judged on the draft's scope and on where the
// agent lives now, before any precondition.
package drafts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
)

var (
	// ErrNotFound is no draft, or one the caller may not reach: on the
	// draft's scope or where its agent lives now. The two answer the same.
	ErrNotFound = errors.New("drafts: no such draft")
	// ErrForbidden is a scope the caller chose and may not publish into; a
	// ForbiddenError says which.
	ErrForbidden = errors.New("drafts: the caller may not publish into that scope")
	// ErrAgentExists is an identity given for an agent already published:
	// its draft opens from its current version, with no identity.
	ErrAgentExists = errors.New("drafts: the agent is already published; open its draft without an identity")
	// ErrPreconditionRequired is a write to a draft without If-Match: every
	// operation on a draft names the state it read.
	ErrPreconditionRequired = errors.New("drafts: a change to a draft needs If-Match")
)

// ForbiddenError is ErrForbidden for one scope.
type ForbiddenError struct{ Scope domain.Scope }

func (e *ForbiddenError) Error() string        { return fmt.Sprintf("%v: %s", ErrForbidden, e.Scope) }
func (e *ForbiddenError) Is(target error) bool { return target == ErrForbidden }

// Caller is who writes, and the scopes they may publish into.
type Caller struct {
	By domain.UserID
	// Allow answers whether the caller holds agent:publish in a scope.
	Allow func(domain.Scope) bool
}

// Identity is what the human step at a new agent's birth chooses.
type Identity struct {
	Name  string
	Scope domain.Scope
}

// Opened is a draft as its row holds it.
type Opened struct {
	Agent       domain.AgentID
	DraftID     string
	Draft       draft.Draft
	Missing     []spec.Missing
	Revision    domain.Revision
	BaseVersion domain.VersionID
}

// Store reads and writes drafts.
type Store struct {
	pool *pgxpool.Pool
	// content keeps what each change said, exactly: the size is judged
	// before anything is written, so nothing is ever truncated.
	content *ledger.Content
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, content: ledger.NewContent(pool).WithLimit(0)}
}

// row is a draft's row, read under its lock.
type row struct {
	draftID  string
	scope    domain.Scope
	stored   draft.Stored
	base     string
	revision domain.Revision
}

func readRow(ctx context.Context, tx pgx.Tx, agent domain.AgentID, lock bool) (row, bool, error) {
	query := `select draft_id::text, company_id, area_id, definition, blocks, base_version, revision
		from agent_drafts where agent_id = $1`
	if lock {
		query += ` for update`
	}
	var r row
	var company, area, revision string
	err := tx.QueryRow(ctx, query, string(agent)).Scan(
		&r.draftID, &company, &area, &r.stored.Definition, &r.stored.Blocks, &r.base, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return row{}, false, nil
	}
	if err != nil {
		return row{}, false, fmt.Errorf("drafts: read %s: %w", agent, err)
	}
	r.scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	r.revision = domain.Revision(revision)
	return r, true, nil
}

// reachable is contract 5: the draft's scope, and where the agent lives now
// when it has a version.
func reachable(c Caller, draftScope domain.Scope, current spec.Spec, published bool) bool {
	if !c.Allow(draftScope) {
		return false
	}
	return !published || c.Allow(publishedScope(current))
}

func publishedScope(s spec.Spec) domain.Scope { return domain.Scope{Company: s.Company, Area: s.Area} }

// revisionOf names a draft's state: what it holds, and which draft it is, so
// a draft discarded and opened again with the same content is another tag.
func revisionOf(draftID string, st draft.Stored) domain.Revision {
	return domain.NewRevision("agent-draft", []byte(st.Definition), string(st.Blocks), draftID)
}

func opened(agent domain.AgentID, r row, d draft.Draft) (Opened, error) {
	missing, err := missingOf(d)
	if err != nil {
		return Opened{}, err
	}
	return Opened{
		Agent: agent, DraftID: r.draftID, Draft: d, Missing: missing,
		Revision: r.revision, BaseVersion: domain.VersionID(r.base),
	}, nil
}

func missingOf(d draft.Draft) ([]spec.Missing, error) {
	rendered, err := spec.Render(d.Definition())
	if err != nil {
		return nil, fmt.Errorf("drafts: render: %w", err)
	}
	_, missing, err := spec.ParseDraft("draft", rendered)
	if err != nil {
		return nil, fmt.Errorf("drafts: a stored draft does not parse: %w", err)
	}
	return missing, nil
}

// Get reads a draft the caller may reach.
func (s *Store) Get(ctx context.Context, agent domain.AgentID, c Caller) (Opened, error) {
	var out Opened
	err := s.within(ctx, func(tx pgx.Tx) error {
		current, published, err := spec.CurrentIn(ctx, tx, agent)
		if err != nil {
			return err
		}
		r, found, err := readRow(ctx, tx, agent, false)
		if err != nil {
			return err
		}
		if !found || !reachable(c, r.scope, current, published) {
			return ErrNotFound
		}
		d, err := draft.Decode(r.stored)
		if err != nil {
			return err
		}
		out, err = opened(agent, r, d)
		return err
	})
	return out, err
}

// within runs fn in a transaction that takes the event graph's lock shared
// first, and commits only if fn succeeds.
func (s *Store) within(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("drafts: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := spec.ShareEventGraph(ctx, tx); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("drafts: commit: %w", err)
	}
	return nil
}
