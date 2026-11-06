package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
Reading one configured thing, by the key it is stored under.

The listings read a kind whole, which is right for a screen showing all of it.
A declarative client refreshing N resources makes N of these, so each has to
cost the same however many others exist.
*/

// ErrProviderUnknown means nobody configured a provider under that name.
var ErrProviderUnknown = errors.New("admin: no such provider")

// MCPServerNamed reads one server configured here.
func (i *Integrations) MCPServerNamed(ctx context.Context, name string) (domain.MCPServer, error) {
	row, err := i.settings.Get(ctx, settings.ScopeInstallation, domain.Scope{}, settings.KindMCPServer, name)
	if errors.Is(err, settings.ErrNotFound) {
		return domain.MCPServer{}, fmt.Errorf("%w: %s", ErrMCPServerUnknown, name)
	}
	if err != nil {
		return domain.MCPServer{}, err
	}
	return serverFromRow(row)
}

// ProviderNamed reads one provider.
func (i *Integrations) ProviderNamed(ctx context.Context, name string) (domain.ModelProvider, error) {
	row, err := i.settings.Get(ctx, settings.ScopeInstallation, domain.Scope{}, settings.KindModelProvider, name)
	if errors.Is(err, settings.ErrNotFound) {
		return domain.ModelProvider{}, fmt.Errorf("%w: %s", ErrProviderUnknown, name)
	}
	if err != nil {
		return domain.ModelProvider{}, err
	}
	return providerFromRow(row)
}
