package spec

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// PostgresSealRows reads agent_seals with the claim of each seal's version.
type PostgresSealRows struct {
	pool *pgxpool.Pool
}

// NewSealRows is the database's view of seals.
func NewSealRows(pool *pgxpool.Pool) *PostgresSealRows {
	return &PostgresSealRows{pool: pool}
}

var _ SealRows = (*PostgresSealRows)(nil)

// Seal reads one seal and its version's claim. The id must already be a well-formed
// uuid: ReadSeal refuses anything else before it gets here.
func (p *PostgresSealRows) Seal(ctx context.Context, id string) (SealRow, error) {
	var row SealRow
	var agent, version, company, area string
	var digest *string
	err := p.pool.QueryRow(ctx, `
		select s.agent_id, s.version_id, s.company_id, s.area_id, s.content_ref, d.digest
		from agent_seals s
		join agent_version_digests d using (agent_id, version_id)
		where s.seal_id = $1::uuid`, id).
		Scan(&agent, &version, &company, &area, &row.ContentRef, &digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return SealRow{}, ErrSealNotFound
	}
	if err != nil {
		return SealRow{}, fmt.Errorf("read seal %s: %w", id, err)
	}
	row.Agent, row.Version = domain.AgentID(agent), domain.VersionID(version)
	row.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	if digest != nil {
		row.Digest = *digest
	}
	return row, nil
}
