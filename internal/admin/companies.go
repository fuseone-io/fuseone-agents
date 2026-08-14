package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

/*
The companies an installation governs.

Until now there was one, created by the bootstrap and named `default`, and
every screen quietly assumed it. What makes more than one a governance question
rather than a table is that creating a company is authority nobody holds
*inside* a company — see domain.Installation — and that a company created
without a grant in it is invisible to the person who created it.

That last part is the same defect as registering an area in an unknown company,
one level up, so it is not left to the caller: creating a company grants the
creator in it, in the same transaction, and the trail records both halves.
*/

var (
	ErrCompanyExists = errors.New("admin: a company with that identifier already exists")
	ErrNoSuchCompany = errors.New("admin: no such company")
)

// Company is one, as an operator sees it.
type Company struct {
	ID domain.CompanyID
	// Label is what people read. The identifier reaches URLs, settings keys
	// and every scope written as "company/area", so it never changes; this
	// does.
	Label     string
	Areas     int
	CreatedAt time.Time
	CreatedBy domain.UserID
	Archived  bool
}

// Companies reads and writes them, recording each change.
type Companies struct{ pool *pgxpool.Pool }

func NewCompanies(pool *pgxpool.Pool) *Companies { return &Companies{pool: pool} }

// List answers with every company, withdrawn ones included.
//
// Included because withdrawing one leaves its runs readable and somebody
// looking at those has to be able to find out what that company was. The row
// says it is archived rather than the listing hiding it.
func (c *Companies) List(ctx context.Context) ([]Company, error) {
	rows, err := c.pool.Query(ctx, `
		select c.company_id, c.name, c.created_at, c.created_by,
		       c.archived_at is not null,
		       (select count(*) from scopes s where s.company_id = c.company_id)
		from companies c
		order by c.company_id`)
	if err != nil {
		return nil, fmt.Errorf("admin: list companies: %w", err)
	}
	defer rows.Close()

	var out []Company
	for rows.Next() {
		var (
			company        Company
			id, label, who string
		)
		if err := rows.Scan(&id, &label, &company.CreatedAt, &who,
			&company.Archived, &company.Areas); err != nil {
			return nil, err
		}
		company.ID, company.Label = domain.CompanyID(id), label
		company.CreatedBy = domain.UserID(who)
		out = append(out, company)
	}
	return out, rows.Err()
}

/*
Create registers a company and grants its creator inside it.

One transaction, and the grant is not optional. A company created without one
is a company its creator cannot see: every listing on this platform is filtered
by the scopes a caller holds, so the screen would report success and then show
nothing — which is exactly the defect this platform already had one level down,
with areas.

The role granted is the creator's own authority over that company rather than a
lesser one. Somebody who may create a company may govern it; handing them
something narrower would mean immediately granting themselves the rest, from a
screen they could not yet see.
*/
func (c *Companies) Create(
	ctx context.Context, id domain.CompanyID, label string, by domain.UserID,
) (Company, error) {
	if err := domain.ValidCompanyID(string(id)); err != nil {
		return Company{}, err
	}
	if strings.TrimSpace(label) == "" {
		label = string(id)
	}

	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return Company{}, fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		insert into companies (company_id, name, created_by) values ($1, $2, $3)
		on conflict (company_id) do nothing`,
		string(id), label, string(by))
	if err != nil {
		return Company{}, fmt.Errorf("admin: create company: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Company{}, fmt.Errorf("%w: %s", ErrCompanyExists, id)
	}

	if _, err := tx.Exec(ctx, `
		insert into role_grants (principal_id, company_id, area_id, role)
		values ($1, $2, '', 'curator')
		on conflict do nothing`, string(by), string(id)); err != nil {
		return Company{}, fmt.Errorf("admin: grant the creator: %w", err)
	}

	if err := Record(ctx, tx, Event{
		Principal: by, Scope: domain.Scope{Company: id},
		Action: "company.created", Target: string(id),
		Detail: map[string]any{"label": label, "granted": string(by)},
	}); err != nil {
		return Company{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Company{}, fmt.Errorf("admin: commit: %w", err)
	}

	return Company{ID: id, Label: label, CreatedBy: by}, nil
}

// Rename changes what a company is called, never what it is keyed by.
func (c *Companies) Rename(
	ctx context.Context, id domain.CompanyID, label string, by domain.UserID,
) error {
	if strings.TrimSpace(label) == "" {
		return fmt.Errorf("admin: a company needs a name")
	}
	return c.recording(ctx, by, id, "company.renamed",
		map[string]any{"label": label},
		`update companies set name = $2 where company_id = $1`, label)
}

/*
Archive withdraws a company from what is offered for new work.

Never a delete. Its runs, decisions and trail name it, and removing the row
would leave every one of them pointing at nothing — a record that cannot say
which company an act belonged to is not the record this platform claims to
keep.
*/
func (c *Companies) Archive(
	ctx context.Context, id domain.CompanyID, by domain.UserID,
) error {
	return c.recording(ctx, by, id, "company.archived", nil,
		`update companies set archived_at = now() where company_id = $1 and archived_at is null`)
}

// Restore puts an archived company back.
func (c *Companies) Restore(
	ctx context.Context, id domain.CompanyID, by domain.UserID,
) error {
	return c.recording(ctx, by, id, "company.restored", nil,
		`update companies set archived_at = null where company_id = $1`)
}

// recording runs one statement and records that somebody ran it.
func (c *Companies) recording(
	ctx context.Context, by domain.UserID, id domain.CompanyID,
	action string, detail map[string]any, sql string, args ...any,
) error {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, sql, append([]any{string(id)}, args...)...)
	if err != nil {
		return fmt.Errorf("admin: %s: %w", action, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrNoSuchCompany, id)
	}

	if err := Record(ctx, tx, Event{
		Principal: by, Scope: domain.Scope{Company: id},
		Action: action, Target: string(id), Detail: detail,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
