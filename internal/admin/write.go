// The one path a configuration change takes.
//
// Storing the setting and recording who changed it happen in one transaction,
// so an installation can never hold a configuration nobody can account for.
// Shared by every administration store rather than written per store: two
// copies of this is how one of them stops writing to the trail.
package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

func writeSetting(
	ctx context.Context, pool *pgxpool.Pool, store *settings.Store,
	by domain.UserID, scope domain.Scope,
	set settings.Setting, action, target string, detail any,
) error {
	return writeFolded(ctx, pool, store, folded{
		by: by, scope: scope, action: action, target: target,
		set: set, detail: detail,
	})
}

/*
folded is a write that may need to see what is stored before it decides.

Some fields mean "leave what is there" when a request omits them — a
credential, a chosen surface. Read before the transaction, that fold is a lost
update waiting for two people: one narrows a server, the other saves a token
having read the older value, and the second commit puts the older value back.
So the reading happens inside, under the row's own lock.
*/
type folded struct {
	by     domain.UserID
	scope  domain.Scope
	action string
	target string
	set    settings.Setting
	detail any
	// fold sees what is stored and returns what to write. Absent for a write
	// that depends on nothing.
	fold func(stored settings.Setting) (settings.Setting, any, error)
}

func writeFolded(
	ctx context.Context, pool *pgxpool.Pool, store *settings.Store, w folded,
) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	set, detail := w.set, w.detail
	if w.fold != nil {
		stored, err := store.RevealTx(ctx, tx,
			set.ScopeKind, set.Scope, set.Kind, set.Name)
		switch {
		case errors.Is(err, settings.ErrNotFound):
			// The first write. Nothing to fold onto is exactly what it looks
			// like, and only this error means it — every other one is a read
			// that did not happen.
			stored = settings.Setting{}
		case err != nil:
			return fmt.Errorf("admin: read what is stored for %s: %w", set.Name, err)
		}
		if set, detail, err = w.fold(stored); err != nil {
			return err
		}
	}

	if err := store.PutTx(ctx, tx, set); err != nil {
		return err
	}
	by, scope, action, target := w.by, w.scope, w.action, w.target
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: scope, Action: action, Target: target, Detail: detail,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// removeSetting deletes an installation-wide setting and records who did.
func removeSetting(
	ctx context.Context, pool *pgxpool.Pool, store *settings.Store,
	by domain.UserID, scope domain.Scope,
	kind settings.Kind, name, action string,
) error {
	return removeScopedSetting(ctx, pool, store, by,
		settings.ScopeInstallation, domain.Scope{}, scope, kind, name, action)
}

/*
removeScopedSetting deletes a setting that lives at a scope of its own.

Separate because deleting is keyed by where the setting is stored while the
trail is keyed by what the change was about, and for most settings those are
different things: an installation-wide provider changed on behalf of one area
is recorded against that area. A conversation is the case where they are the
same, and where assuming installation would delete the wrong key and report
success.
*/
func removeScopedSetting(
	ctx context.Context, pool *pgxpool.Pool, store *settings.Store,
	by domain.UserID, at settings.ScopeKind, stored, recorded domain.Scope,
	kind settings.Kind, name, action string,
) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := store.DeleteTx(ctx, tx, at, stored, kind, name); err != nil {
		return err
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: recorded, Action: action, Target: name,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
