package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
Removing somebody who signs in with a password.

Deactivated and taken off the list, never deleted. The trail and every run name
people by id, and an audit whose ids resolve to nothing cannot be read. What
has to stop is everything they could still do: the sign-in, the sessions
already open and the grants, all in one transaction so there is no moment in
which one of them still works.

Only accounts this installation made. Somebody a provider vouched for comes
back at their next sign-in, so removing them here would be a screen that
lies; the group in the provider is what to change.
*/

var (
	// ErrNotLocal means the principal does not sign in with a password made
	// here: a provider would bring them back, a machine has its own door.
	ErrNotLocal = errors.New("auth: only an account made here can be removed here")
	// ErrRemovingYourself means the caller named their own account.
	ErrRemovingYourself = errors.New("auth: you cannot remove your own account")
	// ErrLastAdministrator means nobody else could still administer the
	// installation, and nobody would be left to grant it again.
	ErrLastAdministrator = errors.New("auth: the last installation administrator cannot be removed")
)

// localProviders are the accounts this installation made itself: the ones an
// administrator created, and the one the setup token did. Neither comes back
// through a sign-in once it is gone.
var localProviders = []string{"local", "bootstrap"}

// removalLock serialises removals, so two administrators removing each other
// at the same moment cannot both find the other still there.
const removalLock = "auth.remove-person"

// Removal names who goes, who decided it and when.
type Removal struct {
	PrincipalID string
	By          string
	At          time.Time
}

// RemoveLocal deactivates an account made here, signs it out everywhere and
// takes away what it held.
func (p *Postgres) RemoveLocal(ctx context.Context, r Removal) error {
	if r.PrincipalID == r.By {
		return ErrRemovingYourself
	}
	return p.onTrail(ctx, func(tx pgx.Tx) error {
		// Locked before anything is judged: a count read before the wait would
		// be a count of somebody who may already be gone.
		if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtext($1))`, removalLock); err != nil {
			return fmt.Errorf("auth: lock removals: %w", err)
		}
		if err := removable(ctx, tx, r.PrincipalID); err != nil {
			return err
		}
		return deactivate(ctx, tx, r)
	})
}

func removable(ctx context.Context, tx pgx.Tx, principalID string) error {
	var local bool
	err := tx.QueryRow(ctx, `
		select kind = 'user' and provider = any($2) from principals
		where principal_id = $1 and disabled_at is null
		for update`, principalID, localProviders).Scan(&local)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoPrincipal
	}
	if err != nil {
		return fmt.Errorf("auth: read principal: %w", err)
	}
	if !local {
		return ErrNotLocal
	}
	return notLastAdministrator(ctx, tx, principalID)
}

// notLastAdministrator counts the people who could still administer the
// installation without this one: anybody active, of any origin, holding
// admin at the installation scope.
func notLastAdministrator(ctx context.Context, tx pgx.Tx, principalID string) error {
	var holds, others bool
	err := tx.QueryRow(ctx, `
		select
		  exists(select 1 from role_grants
		         where principal_id = $1 and company_id = $2 and area_id = '' and role = $3),
		  exists(select 1 from role_grants g
		         join principals p on p.principal_id = g.principal_id
		         where g.company_id = $2 and g.area_id = '' and g.role = $3
		           and p.kind = 'user' and p.disabled_at is null and p.principal_id <> $1)`,
		principalID, string(domain.Installation), string(domain.RoleAdmin)).Scan(&holds, &others)
	if err != nil {
		return fmt.Errorf("auth: count administrators: %w", err)
	}
	if holds && !others {
		return ErrLastAdministrator
	}
	return nil
}

func deactivate(ctx context.Context, tx pgx.Tx, r Removal) error {
	steps := []struct {
		what  string
		query string
		args  []any
	}{
		{"disable", `update principals set disabled_at = $2 where principal_id = $1`,
			[]any{r.PrincipalID, r.At}},
		{"revoke sessions", `update sessions set revoked_at = $2 where principal_id = $1 and revoked_at is null`,
			[]any{r.PrincipalID, r.At}},
		{"remove grants", `delete from role_grants where principal_id = $1`,
			[]any{r.PrincipalID}},
		// Ids only: who removed whom is the whole of the fact.
		{"record", `insert into admin_events (principal_id, action, target) values ($1, 'person.removed', $2)`,
			[]any{r.By, r.PrincipalID}},
	}
	for _, step := range steps {
		if _, err := tx.Exec(ctx, step.query, step.args...); err != nil {
			return fmt.Errorf("auth: %s: %w", step.what, err)
		}
	}
	return nil
}
