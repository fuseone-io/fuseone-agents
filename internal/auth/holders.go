package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
Somebody must always be able to manage identity over the whole installation.

That is whoever is active and holds, at the installation scope, a role that
carries identity:write — derived from the role table rather than named, so a
role that gains or loses it is counted the day it does. A provider's grant
counts like one made here: either way it is somebody who can get in and fix
things. Without one, the only way back is a setup token and a shell.

Two doors can take it away, removing a person and replacing their grants, and
both judge the state their change produces rather than predicting it.

Lock order, the same at both doors: the identity lock first, then the
principal's row. The lock serialises every change to who holds identity, so
the count one change reads cannot be made stale by another committing beside
it; taken before the row, two doors never hold one each while waiting for the
other. An identity provider's sign-in (ReplaceAssertedGrants) takes neither and
is not judged: what a provider asserts is its own authority, and a sign-in
cannot be refused for what a group decided.
*/

// ErrLastAdministrator means the change would leave nobody able to manage
// identity over the installation.
var ErrLastAdministrator = errors.New("auth: nobody would be left to manage identity over the installation")

// ErrRemoved means the person was removed: the row stays for the trail, and
// nothing more is done to it.
var ErrRemoved = errors.New("auth: this person was removed")

const identityLock = "auth.identity-holders"

func lockIdentity(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtext($1))`, identityLock); err != nil {
		return fmt.Errorf("auth: lock identity: %w", err)
	}
	return nil
}

type lockedPrincipal struct {
	kind     string
	provider string
	disabled bool
}

// lockPrincipal locks one principal's row, after the identity lock, and refuses
// a person who was removed. A disabled machine is not removed: disabling one is
// reversible, and what it holds is part of what comes back.
func lockPrincipal(ctx context.Context, tx pgx.Tx, principalID string) (lockedPrincipal, error) {
	var p lockedPrincipal
	err := tx.QueryRow(ctx, `
		select kind, provider, disabled_at is not null from principals
		where principal_id = $1 for update`, principalID).Scan(&p.kind, &p.provider, &p.disabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNoPrincipal
	}
	if err != nil {
		return p, fmt.Errorf("auth: read principal: %w", err)
	}
	if p.disabled && p.kind == string(domain.PrincipalUser) {
		return p, ErrRemoved
	}
	return p, nil
}

// keepingAnAdministrator applies a change and refuses it when it took the
// installation from somebody able to manage identity to nobody. An
// installation with nobody yet is not refused: its first grants have to go in.
func keepingAnAdministrator(ctx context.Context, tx pgx.Tx, change func() error) error {
	before, err := identityHolders(ctx, tx)
	if err != nil {
		return err
	}
	if err := change(); err != nil {
		return err
	}
	after, err := identityHolders(ctx, tx)
	if err != nil {
		return err
	}
	if before > 0 && after == 0 {
		return ErrLastAdministrator
	}
	return nil
}

func identityHolders(ctx context.Context, tx pgx.Tx) (int, error) {
	roles := domain.RolesAllowing(domain.PermIdentityWrite)
	names := make([]string, 0, len(roles))
	for _, r := range roles {
		names = append(names, string(r))
	}
	var n int
	if err := tx.QueryRow(ctx, `
		select count(distinct p.principal_id) from role_grants g
		join principals p on p.principal_id = g.principal_id
		where g.company_id = $1 and g.area_id = '' and g.role = any($2)
		  and p.kind = 'user' and p.disabled_at is null`,
		string(domain.Installation), names).Scan(&n); err != nil {
		return 0, fmt.Errorf("auth: count identity holders: %w", err)
	}
	return n, nil
}
