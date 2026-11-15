package admin

import (
	"context"
	"errors"

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
	switch {
	case kind == settings.ScopeInstallation && stored == (domain.Scope{}):
		return domain.Scope{Company: domain.Installation}, true
	case kind == settings.ScopeCompany && stored.Company != "" && stored.Area == "":
		return stored, true
	case kind == settings.ScopeArea && stored.Company != "" && stored.Area != "":
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

func knownProviderScope(row settings.Setting) domain.Scope {
	scope, _ := providerRuntimeScope(row.ScopeKind, row.Scope)
	return scope
}
