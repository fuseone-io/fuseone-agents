package admin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
ConfiguredProvider is a provider and the credential that goes with it, read
together.

They travel as one value because they were read as one row. Split into two
calls, an edit between them pairs a new credential with an old address, which
is the failure a rotation exists to avoid.
*/
type ConfiguredProvider struct {
	domain.ModelProvider
	APIKey string
	// Unreadable says why a stored credential did not open here. The provider
	// is still listed: its name and its address are true whatever this process
	// can do with its key, and a caller that lost the row would let something
	// else answer under that name.
	Unreadable string
}

/*
ProvidersWithCredentials is the list a process needs to build clients.

One query, credentials opened. The ordinary listing says only that a credential
exists, which is what a screen should know; this is the wiring that has to
speak to the endpoint.
*/
func (i *Integrations) ProvidersWithCredentials(ctx context.Context) ([]ConfiguredProvider, error) {
	rows, err := i.settings.RevealAll(ctx, settings.KindModelProvider)
	if err != nil {
		return nil, err
	}
	out := make([]ConfiguredProvider, 0, len(rows))
	for _, row := range rows {
		// Where it is configured decides which runs reach it (NT-019), and is
		// judged before the value is read. A row of no known scope is held by
		// its name alone, with no scope and no key: it reaches no run, and the
		// environment does not fill its name.
		scope, known := providerRuntimeScope(row.ScopeKind, row.Scope)
		if !known {
			out = append(out, ConfiguredProvider{ModelProvider: domain.ModelProvider{Name: row.Name}})
			continue
		}
		var stored storedProvider
		if err := json.Unmarshal(row.Value, &stored); err != nil {
			return nil, fmt.Errorf("admin: decode provider %s: %w", row.Name, err)
		}
		out = append(out, ConfiguredProvider{
			ModelProvider: domain.ModelProvider{
				Name: row.Name, Kind: stored.Kind, BaseURL: stored.BaseURL,
				Models:               stored.Models,
				AutoToolChoiceModels: stored.AutoToolChoiceModels,
				Enabled:              row.Enabled, HasKey: row.HasSecret,
				Scope:     scope,
				UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt,
			},
			APIKey:     row.Secret,
			Unreadable: row.SecretUnreadable,
		})
	}
	return out, nil
}
