package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
Two things write grants, and they must not overwrite each other.

An identity provider writes on every sign-in: it re-derives what its groups say
and the result has to be authoritative, or leaving a team would leave the
access behind. An operator writes by hand, for a service account or for
somebody the provider's groups do not cover.

They are separated by who granted, which the column has carried since the
schema was written. Without that separation the console would offer an edit
that survives until its holder next signs in — the kind of feature that teaches
people not to trust the screen.
*/

// assertedPrefix marks a grant an identity provider produced.
const assertedPrefix = "oidc:"

func assertedBy(provider string) string { return assertedPrefix + provider }

// Asserted reports whether a grant came from an identity provider.
func Asserted(grantedBy string) bool {
	return strings.HasPrefix(grantedBy, assertedPrefix)
}

// ReplaceAssertedGrants makes one provider's assertion the whole truth about
// what that provider grants, and touches nothing else.
//
// Scoped to the provider because two of them is normal — staff in one,
// partners in another — and signing in through one must not revoke what the
// other asserted.
func (p *Postgres) ReplaceAssertedGrants(
	ctx context.Context, principalID, provider string, grants []domain.Grant,
) error {
	return p.replaceGrants(ctx, principalID, assertedBy(provider), grants,
		`delete from role_grants where principal_id = $1 and granted_by = $2`,
		assertedBy(provider))
}

// SetGrants replaces what an operator granted by hand, leaving every asserted
// grant in place. An operator cannot revoke what a group decides.
func (p *Postgres) SetGrants(
	ctx context.Context, principalID string, grants []domain.Grant, by string,
) error {
	return p.replaceGrants(ctx, principalID, by, grants,
		`delete from role_grants where principal_id = $1 and granted_by not like $2`,
		assertedPrefix+"%")
}

func (p *Postgres) replaceGrants(
	ctx context.Context, principalID, by string, grants []domain.Grant,
	clear string, clearArg any,
) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("auth: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, clear, principalID, clearArg); err != nil {
		return fmt.Errorf("auth: clear grants: %w", err)
	}
	for _, g := range grants {
		if _, err := tx.Exec(ctx, `
			insert into role_grants (principal_id, company_id, area_id, role, granted_by)
			values ($1, $2, $3, $4, $5)
			on conflict do nothing`,
			principalID, string(g.Scope.Company), string(g.Scope.Area),
			string(g.Role), by); err != nil {
			return fmt.Errorf("auth: write grant: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// People lists everybody the installation knows about, with what they hold.
func (p *Postgres) People(ctx context.Context) ([]domain.Person, error) {
	rows, err := p.pool.Query(ctx, `
		select p.principal_id, p.kind, p.display, coalesce(p.email, ''), p.provider,
		       coalesce(p.last_seen_at, 'epoch'::timestamptz), p.disabled_at is not null,
		       coalesce(g.company_id, ''), coalesce(g.area_id, ''),
		       coalesce(g.role, ''), coalesce(g.granted_by, '')
		from principals p
		left join role_grants g on g.principal_id = p.principal_id
		order by p.display, p.principal_id, g.company_id, g.area_id, g.role`)
	if err != nil {
		return nil, fmt.Errorf("auth: list people: %w", err)
	}
	defer rows.Close()

	return scanPeople(rows)
}

func scanPeople(rows pgx.Rows) ([]domain.Person, error) {
	var (
		out   []domain.Person
		index = map[string]int{}
	)
	for rows.Next() {
		var (
			person                    domain.Person
			kind, company, area, role string
			grantedBy                 string
		)
		if err := rows.Scan(&person.ID, &kind, &person.Display, &person.Email,
			&person.Provider, &person.LastSeen, &person.Disabled,
			&company, &area, &role, &grantedBy); err != nil {
			return nil, fmt.Errorf("auth: scan person: %w", err)
		}
		person.Kind = domain.PrincipalKind(kind)

		at, seen := index[person.ID]
		if !seen {
			at = len(out)
			index[person.ID] = at
			out = append(out, person)
		}
		// The left join means a person with no grant still gets a row, with
		// empty columns. Somebody who can sign in and do nothing is exactly
		// who an operator is looking for here, so they must not be dropped.
		if role == "" {
			continue
		}
		out[at].Grants = append(out[at].Grants, domain.HeldGrant{
			Grant: domain.Grant{
				Scope: domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)},
				Role:  domain.Role(role),
			},
			Asserted: Asserted(grantedBy),
			By:       grantedBy,
		})
	}
	return out, rows.Err()
}
