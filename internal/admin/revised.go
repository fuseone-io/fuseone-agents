package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

/*
revised makes a write one a client can reason about: it answers an If-Match, and writing
what is already there changes nothing.

Nothing means nothing, because each of these moves something: updated_at is what a worker
compares to decide a tool server or a Slack socket has to reconnect, so a save with no
change used to restart every session on every worker. A `terraform apply` that changes
nothing must not do that. A real change of configuration or secret still moves it.
*/
type revised struct {
	expect domain.IfMatch
	// of names a row of this resource, secret included.
	of func(set settings.Setting) domain.Revision
	// ofUnreadable names one whose secret will not open.
	ofUnreadable func(set settings.Setting) domain.Revision
	// differs reports a change the revision does not cover. Tokens a worker rotates are
	// left out of a revision, or it would move with nobody having written anything, but
	// a person pasting new ones is a change.
	differs func(stored, candidate settings.Setting) bool
	// lockedByGuard says the write's guard already holds a lock of this resource's own,
	// taken whether the row exists or not, so a second one would only stack writers up.
	lockedByGuard bool
	// replacesUnreadable lets a write go ahead when the stored secret will not open,
	// for a resource whose write does not need it. That is how a key sealed under a
	// lost master key gets replaced.
	replacesUnreadable bool
}

// stored is what the row holds, as far as this write can read it.
type storedRow struct {
	set        settings.Setting
	exists     bool
	unreadable bool
}

func readStored(
	ctx context.Context, tx settings.DB, store *settings.Store, w folded,
) (storedRow, error) {
	set := w.set
	found, err := store.RevealTx(ctx, tx, set.ScopeKind, set.Scope, set.Kind, set.Name)
	switch {
	case err == nil:
		return storedRow{set: found, exists: true}, nil
	case errors.Is(err, settings.ErrNotFound):
		// The first write. Nothing to fold onto is exactly what it looks like, and
		// only this error means it: every other one is a read that did not happen.
		return storedRow{}, nil
	case w.revised != nil && w.revised.replacesUnreadable && (errors.Is(err, vault.ErrCiphertext) || errors.Is(err, settings.ErrNoVault)):
		held, err := store.Get(ctx, set.ScopeKind, set.Scope, set.Kind, set.Name)
		if err != nil {
			return storedRow{}, fmt.Errorf("admin: read what is stored for %s: %w", set.Name, err)
		}
		return storedRow{set: held, exists: true, unreadable: true}, nil
	}
	return storedRow{}, fmt.Errorf("admin: read what is stored for %s: %w", set.Name, err)
}

// judge applies the precondition to what is there now.
func (r *revised) judge(row storedRow) (domain.Revision, error) {
	var current domain.Revision
	switch {
	case !row.exists:
	case row.unreadable:
		current = r.ofUnreadable(row.set)
	default:
		current = r.of(row.set)
	}
	if !r.expect.Allows(row.exists, current) {
		return "", &domain.StaleError{Current: current}
	}
	return current, nil
}

// unchanged reports that writing this would leave the resource as it is. A row whose secret
// will not open is never unchanged: there is no telling what the write would keep.
func (r *revised) unchanged(row storedRow, current domain.Revision, candidate settings.Setting) bool {
	if !row.exists || row.unreadable || r.of(candidate) != current {
		return false
	}
	return r.differs == nil || !r.differs(row.set, candidate)
}

// effective is the row a write would leave, the secret it keeps included.
func effective(set, stored settings.Setting) settings.Setting {
	switch {
	case set.ClearSecret:
		set.Secret = ""
	case set.Secret == "":
		set.Secret = stored.Secret
	}
	return set
}

// writeFolded runs a write that answers no precondition.
func writeFolded(
	ctx context.Context, pool *pgxpool.Pool, store *settings.Store, w folded,
) error {
	_, err := writeRevised(ctx, pool, store, w)
	return err
}

// writeRevised does the write, in this order: the lock, then what the caller may do, then
// what is stored, then the precondition, then whether there is anything to do. A guard's
// effects roll back with a refusal or a no-op.
func writeRevised(
	ctx context.Context, pool *pgxpool.Pool, store *settings.Store, w folded,
) (domain.Revision, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if w.revised != nil && !w.revised.lockedByGuard {
		if err := settings.GuardTx(ctx, tx, w.set); err != nil {
			return "", err
		}
	}
	if w.guard != nil {
		if err := w.guard(ctx, tx); err != nil {
			return "", err
		}
	}

	var row storedRow
	if w.fold != nil || w.revised != nil {
		if row, err = readStored(ctx, tx, store, w); err != nil {
			return "", err
		}
	}
	var current domain.Revision
	if w.revised != nil {
		if current, err = w.revised.judge(row); err != nil {
			return "", err
		}
	}

	set, detail := w.set, w.detail
	if w.fold != nil {
		if set, detail, err = w.fold(row.set); err != nil {
			return "", err
		}
	}

	var result domain.Revision
	if w.revised != nil {
		candidate := effective(set, row.set)
		if w.revised.unchanged(row, current, candidate) {
			return current, nil
		}
		result = w.revised.of(candidate)
	}

	if err := store.PutTx(ctx, tx, set); err != nil {
		return "", err
	}
	if w.then != nil {
		if err := w.then(ctx, tx); err != nil {
			return "", err
		}
	}
	if err := Record(ctx, tx, Event{
		Principal: w.by, Scope: w.scope, Action: w.action, Target: w.target, Detail: detail,
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("admin: commit: %w", err)
	}
	return result, nil
}

// removalGuard holds a removal against the row's writers and answers its precondition.
// With no header it reads nothing, so a row whose secret will not open can still go.
func removalGuard(
	ctx context.Context, tx settings.DB, store *settings.Store, set settings.Setting, r *revised,
) error {
	if !r.lockedByGuard {
		if err := settings.GuardTx(ctx, tx, set); err != nil {
			return err
		}
	}
	if !r.expect.Present() {
		return nil
	}
	row, err := readStored(ctx, tx, store, folded{set: set, revised: r})
	if err != nil {
		return err
	}
	_, err = r.judge(row)
	return err
}
