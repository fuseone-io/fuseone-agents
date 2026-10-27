package domain_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestToolID_platformToolsHaveNoServer(t *testing.T) {
	t.Parallel()

	// A platform-owned tool is served by the process itself. Treating its
	// prefix as an MCP server name would make "$fuseone" a server somebody
	// could expect to be connected, and park runs over it.
	for _, id := range []domain.ToolID{
		domain.ToolMemoryFind, domain.ToolMemorySuggest,
		domain.ToolContextRead, domain.ToolTicketAnswer,
	} {
		if !id.PlatformOwned() {
			t.Errorf("%s: PlatformOwned = false, want true", id)
		}
		if server := id.Server(); server != "" {
			t.Errorf("%s: Server = %q, want none", id, server)
		}
	}
}

func TestToolID_serverIsTheNamespaceBeforeTheFirstDot(t *testing.T) {
	t.Parallel()

	cases := map[domain.ToolID]string{
		"grafana.query_loki_logs": "grafana",
		"crm.lookup":              "crm",
		// Registration namespaces tools as server.remoteName, and a remote
		// name may itself carry dots.
		"gw.apis.list": "gw",
		// Malformed, but must not panic or invent a server.
		"nodots": "",
	}
	for id, want := range cases {
		if id.PlatformOwned() {
			t.Errorf("%s: PlatformOwned = true, want false", id)
		}
		if got := id.Server(); got != want {
			t.Errorf("%s: Server = %q, want %q", id, got, want)
		}
	}
}
