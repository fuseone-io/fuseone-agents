package httpapi

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// The catalogue is what an author picks tools from when building an agent. A
// caller who may read tools in their own area reads it; one who may not read
// tools anywhere does not.
func TestListTools_readsWithToolReadInAnyArea(t *testing.T) {
	t.Parallel()
	s := NewServer(ledger.NewMemory(), "test").
		WithAdministration(nil, &fakeTools{entries: []domain.ToolEntry{{ID: "crm.lookup", Server: "crm"}}}, nil)

	for _, role := range []domain.Role{domain.RoleAuthor, domain.RoleAdmin} {
		resp, err := s.ListTools(inArea("cx", role), openapi.ListToolsRequestObject{})
		if err != nil {
			t.Fatalf("ListTools: %v", err)
		}
		if _, ok := resp.(openapi.ListTools200JSONResponse); !ok {
			t.Errorf("%s in acme/cx: %T, want the catalogue", role, resp)
		}
	}
	// An approver holds no tool:read, in any scope.
	resp, err := s.ListTools(inArea("cx", domain.RoleApprover), openapi.ListToolsRequestObject{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if _, ok := resp.(openapi.ListTools403ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("approver: %T, want 403", resp)
	}
}
