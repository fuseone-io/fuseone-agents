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
// yet published, each in a sealed scope allow accepts, at most CandidateListLimit of them.
func (r *CandidateReader) List(
	ctx context.Context, agent domain.AgentID, allow func(domain.Scope) bool,
) ([]CandidateSeal, error) {
	return r.visible(ctx, candidateSealsSQL, CandidateListLimit, allow, string(agent))
}

/*
Read is a candidate's sealed text, from the newest seal the caller can see whose bytes check
out as the worker checks them, and which seal that was. It is for display: a run stays
pinned to the seal it opened with, whatever this answers.

No visible seal, or a version already published, is ErrCandidateNotFound. Visible seals
that are all unreadable are domain.ErrCandidateUnreadable, which a caller outside every
seal's scope never gets: that would tell them the candidate exists.
*/
func (r *CandidateReader) Read(
	ctx context.Context, agent domain.AgentID, version domain.VersionID, allow func(domain.Scope) bool,
) (Candidate, error) {
	seals, err := r.visible(ctx, candidateVersionSealsSQL, 0, allow, string(agent), string(version))
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

const candidateSealsSQL = `
	select s.seal_id::text, s.version_id, s.company_id, s.area_id, s.sealed_by, s.sealed_at
	from agent_seals s
	where s.agent_id = $1
	  and not exists (select 1 from agent_specs p
	                  where p.agent_id = s.agent_id and p.version_id = s.version_id)
	order by s.sealed_at desc, s.seal_id desc`

const candidateVersionSealsSQL = `
	select s.seal_id::text, s.version_id, s.company_id, s.area_id, s.sealed_by, s.sealed_at
	from agent_seals s
	where s.agent_id = $1 and s.version_id = $2
	  and not exists (select 1 from agent_specs p
	                  where p.agent_id = s.agent_id and p.version_id = s.version_id)
	order by s.sealed_at desc, s.seal_id desc`

// visible streams seals newest first and keeps the ones allow accepts, up to limit (zero
// is all of them).
func (r *CandidateReader) visible(
	ctx context.Context, query string, limit int, allow func(domain.Scope) bool, args ...any,
) ([]CandidateSeal, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read candidate seals: %w", err)
	}
	defer rows.Close()
	var out []CandidateSeal
	for rows.Next() && (limit == 0 || len(out) < limit) {
		seal, err := scanCandidateSeal(rows)
		if err != nil {
			return nil, err
		}
		if allow(seal.Scope) {
			out = append(out, seal)
		}
	}
	return out, rows.Err()
}

func scanCandidateSeal(rows pgx.Rows) (CandidateSeal, error) {
	var seal CandidateSeal
	var version, company, area, by string
	if err := rows.Scan(&seal.SealID, &version, &company, &area, &by, &seal.SealedAt); err != nil {
		return CandidateSeal{}, fmt.Errorf("read candidate seal: %w", err)
	}
	seal.Version, seal.SealedBy = domain.VersionID(version), domain.UserID(by)
	seal.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	return seal, nil
}
