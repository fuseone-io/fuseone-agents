// Package scope stores the areas somebody declared.
//
// Everything else in the platform files work under a scope — an agent, a
// ceiling, a policy's reach, a grant. Until this package existed, the set of
// areas was whatever those rows happened to mention, so an area could be
// created by a typo and a ceiling could govern nothing without anybody being
// told.
package scope

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// ErrNoArea means nothing is registered under that id.
var ErrNoArea = errors.New("scope: no area with that id")

// Store is the registry.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Put registers an area, or relabels the one already there.
//
// The typed name is folded to a canonical id before it is stored, which is
// what makes registering "Risco de Crédito" twice one row rather than two.
// The label is whatever the last writer typed; the id is the platform's and no
// amount of retyping changes it.
func (s *Store) Put(
	ctx context.Context, company domain.CompanyID, typed, label string, by domain.UserID,
) (domain.RegisteredScope, error) {
	area, err := domain.NormalizeAreaID(typed)
	if err != nil {
		return domain.RegisteredScope{}, err
	}
	if company == "" {
		return domain.RegisteredScope{}, errors.New("scope: an area belongs to a company")
	}

	row := s.pool.QueryRow(ctx, `
        insert into scopes (company_id, area_id, label, created_by)
        values ($1, $2, $3, $4)
        on conflict (company_id, area_id) do update
            set label = excluded.label
        returning company_id, area_id, label, created_at, created_by`,
		string(company), string(area), label, string(by))

	return scan(row)
}

// List answers with the areas inside the scopes a caller can reach.
//
// Filtered in the query rather than read and discarded, and empty scopes
// answer with nothing rather than with everything: a caller who reaches
// nowhere must not be handed the installation.
func (s *Store) List(ctx context.Context, visible []domain.Scope) ([]domain.RegisteredScope, error) {
	if len(visible) == 0 {
		return nil, nil
	}

	companies, areas := make([]string, 0, len(visible)), make([]string, 0, len(visible))
	for _, v := range visible {
		companies = append(companies, string(v.Company))
		// An empty area is a grant over the whole company, which reaches every
		// area in it. '*' stands for that here because a NULL in the array
		// would make the comparison NULL and quietly match nothing.
		if v.Area == "" {
			areas = append(areas, "*")
			continue
		}
		areas = append(areas, string(v.Area))
	}

	rows, err := s.pool.Query(ctx, `
        select s.company_id, s.area_id, s.label, s.created_at, s.created_by
        from scopes s
        join unnest($1::text[], $2::text[]) as held(company_id, area_id)
            on held.company_id = s.company_id
           and (held.area_id = '*' or held.area_id = s.area_id)
        group by s.company_id, s.area_id, s.label, s.created_at, s.created_by
        order by s.company_id, s.area_id`,
		companies, areas)
	if err != nil {
		return nil, fmt.Errorf("scope: list: %w", err)
	}
	defer rows.Close()

	var out []domain.RegisteredScope
	for rows.Next() {
		got, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, got)
	}
	return out, rows.Err()
}

// Delete withdraws an area from the registry.
//
// It does not touch the agents, ceilings or policies filed under it. Those
// rows keep naming the area, and the area keeps meaning what it meant — the
// registry stops offering it for new work, which is a different thing from
// rewriting history.
func (s *Store) Delete(ctx context.Context, company domain.CompanyID, area domain.AreaID) error {
	tag, err := s.pool.Exec(ctx,
		`delete from scopes where company_id = $1 and area_id = $2`,
		string(company), string(area))
	if err != nil {
		return fmt.Errorf("scope: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoArea
	}
	return nil
}

type scanner interface{ Scan(dest ...any) error }

func scan(row scanner) (domain.RegisteredScope, error) {
	var got domain.RegisteredScope
	var company, area, by string
	if err := row.Scan(&company, &area, &got.Label, &got.CreatedAt, &by); err != nil {
		return domain.RegisteredScope{}, fmt.Errorf("scope: read: %w", err)
	}
	got.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	got.CreatedBy = domain.UserID(by)
	return got, nil
}
