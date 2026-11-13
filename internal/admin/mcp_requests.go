package admin

import (
	"encoding/json"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

// MCPServerRequest is a probe or a rediscovery a person asked for, with where
// the server it was asked of was stored. A worker answers it only while the
// server it holds under the name is still stored there (NT-019).
type MCPServerRequest struct {
	Name       string
	Scope      domain.Scope
	Generation int64
}

// requestedAt is what a probe request stores about the row it was asked of.
type requestedAt struct {
	ScopeKind string `json:"scopeKind,omitempty"`
	Company   string `json:"company,omitempty"`
	Area      string `json:"area,omitempty"`
}

// probeRequestValue is what a probe request stores about the row it names.
func probeRequestValue(kind settings.ScopeKind, stored domain.Scope) json.RawMessage {
	raw, err := json.Marshal(requestedAt{
		ScopeKind: string(kind), Company: string(stored.Company), Area: string(stored.Area),
	})
	if err != nil {
		// Three strings always encode; the installation's is the safe reading.
		return json.RawMessage(`{}`)
	}
	return raw
}

// requestedScope is the runtime scope a stored probe request names. One stored
// before requests named a row was about the installation's server, the only
// place servers were stored then.
func requestedScope(raw []byte) domain.Scope {
	var at requestedAt
	if len(raw) == 0 || json.Unmarshal(raw, &at) != nil || at.ScopeKind == "" {
		return domain.Scope{Company: domain.Installation}
	}
	return storedRuntimeScope(at.ScopeKind, at.Company, at.Area)
}

// storedAt is where a server of the given runtime scope is stored: the
// reverse of mcpServerRuntimeScope. Zero is a server no row describes, a
// --mcp flag's, which is the installation's.
func storedAt(runtime domain.Scope) (settings.ScopeKind, domain.Scope) {
	switch {
	case runtime == domain.Scope{} || runtime.Company == domain.Installation:
		return settings.ScopeInstallation, domain.Scope{}
	case runtime.Area != "":
		return settings.ScopeArea, runtime
	default:
		return settings.ScopeCompany, runtime
	}
}
