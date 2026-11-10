package httpapi

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// The version comparison, at each of its doors: either side named, and the version before
// picked when none is (trust_scope_test.go has the scenario).

func TestCompareVersions_aVersionPublishedInAnotherScope_isNotFound(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		params openapi.CompareVersionsParams
	}{
		{"named as from", openapi.CompareVersionsParams{From: ptr("v1"), To: ptr("v2")}},
		{"named as to", openapi.CompareVersionsParams{From: ptr("v2"), To: ptr("v1")}},
		{"picked as the version before", openapi.CompareVersionsParams{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			store := ledger.NewMemory()
			server := comparable(t, store).WithAgents(republishedInCx(t, store))

			resp, err := server.CompareVersions(inArea("cx", domain.RoleAuthor),
				openapi.CompareVersionsRequestObject{AgentId: "triage", Params: c.params})
			if err != nil {
				t.Fatalf("CompareVersions: %v", err)
			}
			if _, ok := resp.(openapi.CompareVersions404ApplicationProblemPlusJSONResponse); !ok {
				t.Errorf("response = %T %+v, want the finance version hidden from cx", resp, resp)
			}
		})
	}
}
