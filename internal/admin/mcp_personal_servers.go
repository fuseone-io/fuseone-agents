package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

// MCPPersonalCredentialServers lists the remote servers a person may hold a
// personal credential for, given the scopes they read tools in (NT-019).
//
// The rule is the write's (personalCredentialTarget): a server whose scope
// one of theirs reaches, either way, reached over HTTP and without a GitHub
// App, which signs as the installation. An area's author reads tools in the
// area and cannot list the servers' configuration, so this is how they find
// the server their agent needs a credential for. No address is answered.
func (i *Integrations) MCPPersonalCredentialServers(
	ctx context.Context, visible []domain.Scope,
) ([]domain.MCPCredentialServer, error) {
	rows, err := i.settings.List(ctx, settings.KindMCPServer)
	if err != nil {
		return nil, err
	}
	out := make([]domain.MCPCredentialServer, 0, len(rows))
	for _, row := range rows {
		offered, err := personallyCredentialed(row, visible)
		if err != nil {
			return nil, err
		}
		if offered {
			out = append(out, domain.MCPCredentialServer{Name: row.Name, Scope: row.Scope, Shared: row.HasSecret})
		}
	}
	slices.SortFunc(out, func(a, b domain.MCPCredentialServer) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func personallyCredentialed(row settings.Setting, visible []domain.Scope) (bool, error) {
	runtime := mcpServerRuntimeScope(row.ScopeKind, row.Scope)
	reached := func(v domain.Scope) bool { return runtime.Contains(v) || v.Contains(runtime) }
	if !slices.ContainsFunc(visible, reached) {
		return false, nil
	}
	var stored storedServer
	if len(row.Value) > 0 {
		if err := json.Unmarshal(row.Value, &stored); err != nil {
			return false, fmt.Errorf("admin: decode MCP server %s: %w", row.Name, err)
		}
	}
	return stored.Transport == domain.TransportHTTP && stored.GitHubApp == nil, nil
}
