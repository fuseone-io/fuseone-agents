package httpapi

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
An area's administrator connects the area's own servers (NT-019), and the
catalogue is where connecting one starts. A recipe names a server other people
publish, nothing of this installation, so whoever may connect a server
somewhere reads it: the act and the screen that starts it share a permission.
*/
func TestListRecipes_anAreaThatConnectsServers_readsTheCatalogue(t *testing.T) {
	t.Parallel()

	for _, role := range []domain.Role{domain.RoleAdmin, domain.RoleCurator} {
		resp, err := NewServer(ledger.NewMemory(), "test").
			ListRecipes(inArea("cx", role), openapi.ListRecipesRequestObject{})
		if err != nil {
			t.Fatalf("ListRecipes(%s): %v", role, err)
		}
		if _, ok := resp.(openapi.ListRecipes200JSONResponse); !ok {
			t.Errorf("%s of an area: response = %T, want the catalogue", role, resp)
		}
	}
}

// Reading tools in an area is not connecting servers there.
func TestListRecipes_anAreaThatCannotConnectServers_isRefused(t *testing.T) {
	t.Parallel()

	for _, role := range []domain.Role{domain.RoleAuthor, domain.RoleAuditor} {
		resp, err := NewServer(ledger.NewMemory(), "test").
			ListRecipes(inArea("cx", role), openapi.ListRecipesRequestObject{})
		if err != nil {
			t.Fatalf("ListRecipes(%s): %v", role, err)
		}
		if _, refused := resp.(openapi.ListRecipes403ApplicationProblemPlusJSONResponse); !refused {
			t.Errorf("%s of an area: response = %T, want 403", role, resp)
		}
	}
}
