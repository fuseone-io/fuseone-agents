package spec

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// ErrCandidateNotFound is a candidate the caller cannot see: no seal of the version in a
// scope they may publish in, or a version that is already published.
var ErrCandidateNotFound = errors.New("no such candidate")

// CandidateSeal is one seal of a candidate, as a listing shows it.
type CandidateSeal struct {
	SealID   string
	Version  domain.VersionID
	Scope    domain.Scope
	SealedBy domain.UserID
	SealedAt time.Time
}

// Candidate is a candidate's sealed text and the seal it was read from.
type Candidate struct {
	CandidateSeal
	Definition string
}

// CandidateListLimit is how many seals one listing answers at most, newest first.
const CandidateListLimit = 200

// CandidateReader lists and reads candidates for display (NT-018 phase 2).
type CandidateReader struct {
	pool    *pgxpool.Pool
	content Content
}

// NewCandidateReader reads candidates from the database and the content store.
func NewCandidateReader(pool *pgxpool.Pool, content Content) *CandidateReader {
	return &CandidateReader{pool: pool, content: content}
}

// List is the caller's candidate seals of agent, newest first: every seal of a version not
// yet published, in a sealed scope one of visible covers, at most CandidateListLimit of
// them. The scope filter and the limit are the query's, so a caller who may see nothing
// costs an index probe, not the agent's whole history.
func (r *CandidateReader) List(
	ctx context.Context, agent domain.AgentID, visible []domain.Scope,
) ([]CandidateSeal, error) {
	if len(visible) == 0 {
		return nil, nil
	}
	companies, areas := scopeArrays(visible)
	return r.seals(ctx, candidateSealsSQL, string(agent), companies, areas,
		string(domain.Installation), CandidateListLimit)
}

/*
Read is a candidate's sealed text, from the newest seal in a scope one of visible covers
whose bytes check out as the worker checks them, and which seal that was. It is for
display: a run stays pinned to the seal it opened with, whatever this answers.

No visible seal, or a version already published, is ErrCandidateNotFound. Visible seals
that are all unreadable are domain.ErrCandidateUnreadable, which a caller outside every
seal's scope never gets: that would tell them the candidate exists.
*/
func (r *CandidateReader) Read(
	ctx context.Context, agent domain.AgentID, version domain.VersionID, visible []domain.Scope,
) (Candidate, error) {
	if len(visible) == 0 {
		return Candidate{}, ErrCandidateNotFound
	}
	companies, areas := scopeArrays(visible)
	seals, err := r.seals(ctx, candidateVersionSealsSQL, string(agent), companies, areas,
		string(domain.Installation), string(version))
	if err != nil {
		return Candidate{}, err
	}
	if len(seals) == 0 {
		return Candidate{}, ErrCandidateNotFound
	}
	rows := NewSealRows(r.pool)
	for _, seal := range seals {
		sealed, err := ReadSeal(ctx, rows, r.content, seal.SealID, SealExpectation{
			Agent: agent, Version: version, Scope: seal.Scope,
		})
		if errors.Is(err, domain.ErrCandidateUnreadable) {
			continue
		}
		if err != nil {
			return Candidate{}, err
		}
		return Candidate{CandidateSeal: seal, Definition: string(sealed.Bytes)}, nil
	}
	return Candidate{}, fmt.Errorf("candidate %s/%s: %w", agent, version, domain.ErrCandidateUnreadable)
}

// visibleSealSQL is domain.Scope.Contains as a predicate over a seal s: a grant over the
// installation ($4, with no area), over the seal's company, or over its area.
const visibleSealSQL = `exists (
		select 1 from unnest($2::text[], $3::text[]) granted(company, area)
		where (granted.company = $4 and granted.area = '')
		   or (granted.company = s.company_id and (granted.area = '' or granted.area = s.area_id)))`

const candidateSealsSQL = `
	select s.seal_id::text, s.version_id, s.company_id, s.area_id, s.sealed_by, s.sealed_at
	from agent_seals s
	where s.agent_id = $1 and ` + visibleSealSQL + `
	  and not exists (select 1 from agent_specs p
	                  where p.agent_id = s.agent_id and p.version_id = s.version_id)
	order by s.sealed_at desc, s.seal_id desc
	limit $5`

const candidateVersionSealsSQL = `
	select s.seal_id::text, s.version_id, s.company_id, s.area_id, s.sealed_by, s.sealed_at
	from agent_seals s
	where s.agent_id = $1 and s.version_id = $5 and ` + visibleSealSQL + `
	  and not exists (select 1 from agent_specs p
	                  where p.agent_id = s.agent_id and p.version_id = s.version_id)
	order by s.sealed_at desc, s.seal_id desc`

const candidateSealByIDSQL = `
	select s.seal_id::text, s.version_id, s.company_id, s.area_id, s.sealed_by, s.sealed_at
	from agent_seals s
	where s.agent_id = $1 and s.seal_id = $5::uuid and ` + visibleSealSQL + `
	  and not exists (select 1 from agent_specs p
	                  where p.agent_id = s.agent_id and p.version_id = s.version_id)`

// scopeArrays is visible as the two parallel arrays the predicate unnests.
func scopeArrays(visible []domain.Scope) ([]string, []string) {
	companies, areas := make([]string, len(visible)), make([]string, len(visible))
	for i, scope := range visible {
		companies[i], areas[i] = string(scope.Company), string(scope.Area)
	}
	return companies, areas
}

// seals reads the seals a query answers, in its order.
func (r *CandidateReader) seals(ctx context.Context, query string, args ...any) ([]CandidateSeal, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read candidate seals: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (CandidateSeal, error) {
		return scanCandidateSeal(row)
	})
}

func scanCandidateSeal(rows pgx.CollectableRow) (CandidateSeal, error) {
	var seal CandidateSeal
	var version, company, area, by string
	if err := rows.Scan(&seal.SealID, &version, &company, &area, &by, &seal.SealedAt); err != nil {
		return CandidateSeal{}, fmt.Errorf("read candidate seal: %w", err)
	}
	seal.Version, seal.SealedBy = domain.VersionID(version), domain.UserID(by)
	seal.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	return seal, nil
}

/*
Seal is one candidate seal of agent, by its id, under the listing's own rule: sealed in a
scope one of visible covers, of a version not yet published. Found whatever page of the
listing it would fall on. Anything else — another agent's seal, one outside every visible
scope, a version published since, an id that is not a seal's — is ErrCandidateNotFound.
*/
func (r *CandidateReader) Seal(
	ctx context.Context, agent domain.AgentID, seal string, visible []domain.Scope,
) (CandidateSeal, error) {
	if len(visible) == 0 || !sealIDShape.MatchString(seal) {
		return CandidateSeal{}, ErrCandidateNotFound
	}
	companies, areas := scopeArrays(visible)
	found, err := r.seals(ctx, candidateSealByIDSQL, string(agent), companies, areas,
		string(domain.Installation), seal)
	if err != nil {
		return CandidateSeal{}, err
	}
	if len(found) == 0 {
		return CandidateSeal{}, ErrCandidateNotFound
	}
	return found[0], nil
}
