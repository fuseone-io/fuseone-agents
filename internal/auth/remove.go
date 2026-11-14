package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
)

// localProviders are the accounts this installation made itself: the ones an
// administrator created, and the one the setup token did. Neither comes back
// through a sign-in once it is gone.
var localProviders = []string{"local", "bootstrap"}

// Removal names who goes, who decided it and when.
type Removal struct {
	PrincipalID string
	By          string
	At          time.Time
}

// RemoveLocal deactivates an account made here, signs it out everywhere and
// takes away what it held. Locks in the order holders.go sets out.
func (p *Postgres) RemoveLocal(ctx context.Context, r Removal) error {
	if r.PrincipalID == r.By {
		return ErrRemovingYourself
	}
	return p.onTrail(ctx, func(tx pgx.Tx) error {
		if err := lockIdentity(ctx, tx); err != nil {
			return err
		}
		locked, err := lockPrincipal(ctx, tx, r.PrincipalID)
		if err != nil {
			return err
		}
		if locked.kind != string(domain.PrincipalUser) || !slices.Contains(localProviders, locked.provider) {
			return ErrNotLocal
		}
		return keepingAnAdministrator(ctx, tx, func() error { return deactivate(ctx, tx, r) })
	})
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
