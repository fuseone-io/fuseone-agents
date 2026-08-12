// The one path a configuration change takes.
//
// Storing the setting and recording who changed it happen in one transaction,
// so an installation can never hold a configuration nobody can account for.
// Shared by every administration store rather than written per store: two
// copies of this is how one of them stops writing to the trail.
package admin

import (
	"context"
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
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := store.PutTx(ctx, tx, set); err != nil {
		return err
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: scope, Action: action, Target: target, Detail: detail,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func removeSetting(
	ctx context.Context, pool *pgxpool.Pool, store *settings.Store,
	by domain.UserID, scope domain.Scope,
	kind settings.Kind, name, action string,
) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := store.DeleteTx(ctx, tx, settings.ScopeInstallation, domain.Scope{}, kind, name); err != nil {
		return err
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: scope, Action: action, Target: name,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
