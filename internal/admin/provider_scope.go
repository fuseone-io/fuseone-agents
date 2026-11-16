package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

// errProviderScopeUnknown is a provider row of no scope this binary knows.
var errProviderScopeUnknown = errors.New("admin: a provider stored at a scope this binary does not know")

/*
providerRuntimeScope is the scope a run must be inside to reach a provider
row (NT-019, phase 3 readers), and whether this binary knows it at all.

Only the three scopes, each with exactly the columns it means. A kind a later
release adds, or columns that contradict the kind, are no scope here: read as
some scope, the row's key would reach runs it was never meant for. Every
provider reader refuses such a row before the writers' release, so no binary
that serves while areas write their own providers can misread one.
*/
func providerRuntimeScope(kind settings.ScopeKind, stored domain.Scope) (domain.Scope, bool) {
	// The installation's sentinel contains every scope: as a company, or an
	// area's company, it would hand the key to every run.
	named := stored.Company != "" && stored.Company != domain.Installation
	switch {
	case kind == settings.ScopeInstallation && stored == (domain.Scope{}):
		return domain.Scope{Company: domain.Installation}, true
	case kind == settings.ScopeCompany && named && stored.Area == "":
		return stored, true
	case kind == settings.ScopeArea && named && stored.Area != "":
		return stored, true
	}
	return domain.Scope{}, false
}

// refuseUnknownProviderScope is the allow a read by name opens a row with:
// one of no known scope is not found, and its key is never opened.
func refuseUnknownProviderScope(kind settings.ScopeKind, stored domain.Scope) error {
	if _, known := providerRuntimeScope(kind, stored); !known {
		return errProviderScopeUnknown
	}
	return nil
}

// Credential opens a provider's key, wherever the provider is stored and under
// its own scope. Separate and explicit: reading configuration is routine,
// reading a credential is not.
func (i *Integrations) Credential(ctx context.Context, name string) (string, error) {
	set, err := i.settings.RevealNamedIf(ctx, settings.KindModelProvider, name, refuseUnknownProviderScope)
	if err != nil {
		return "", err
	}
	return set.Secret, nil
}

// lockProviderName holds every write and removal of one provider name, at any
// scope, until the transaction ends: taken first, before any row, so two
// scopes racing for one name meet here rather than at the unique index.
func lockProviderName(ctx context.Context, tx settings.DB, name string) error {
	if _, err := tx.Exec(ctx,
		`select pg_advisory_xact_lock(hashtextextended('model_provider:' || $1, 0))`, name); err != nil {
		return fmt.Errorf("admin: lock the provider name %s: %w", name, err)
	}
	return nil
}

// providerWriteGuard takes a provider write's locks in the order a removal
// takes them, the name then the row, and answers a name another scope holds
// before anything else (NT-019).
func providerWriteGuard(name string, at settings.ScopeKind, stored domain.Scope) func(context.Context, settings.DB) error {
	return func(ctx context.Context, tx settings.DB) error {
		if err := lockProviderName(ctx, tx, name); err != nil {
			return err
		}
		if err := settings.GuardTx(ctx, tx, settings.Setting{
			ScopeKind: at, Scope: stored, Kind: settings.KindModelProvider, Name: name,
		}); err != nil {
			return err
		}
		return nameOfKindHeldElsewhere(ctx, tx, settings.KindModelProvider, name, at, stored)
	}
}

// ProviderStoredAt is where a provider's row lives, the installation's for a
// name no row holds: where a door authorizes an act on it (NT-019).
func (i *Integrations) ProviderStoredAt(ctx context.Context, name string) (settings.ScopeKind, domain.Scope, error) {
	var kind, company, area string
	err := i.pool.QueryRow(ctx, `
		select scope_kind, company_id, area_id from settings
		where kind = $1 and name = $2`, string(settings.KindModelProvider), name,
	).Scan(&kind, &company, &area)
	if errors.Is(err, pgx.ErrNoRows) {
		return settings.ScopeInstallation, domain.Scope{}, nil
	}
	if err != nil {
		return "", domain.Scope{}, fmt.Errorf("admin: find provider %s: %w", name, err)
	}
	return settings.ScopeKind(kind), domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}, nil
}
