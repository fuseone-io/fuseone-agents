package httpapi

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	memstore "github.com/fuseone/agents/internal/memory"
)

func TestCreateMemoryAssertion_completeLegacyIdentity_isPreservedForCorrection(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	seedFinishedEvidence(t, store, "run-evidence", scope, domain.ScopeLabels(scope), "sha256:answer")

	memory := memstore.NewMemory()
	remember(t, memory, memoryAssertionFixture("cx", "grafana datasource",
		func(a *domain.MemoryAssertion) {
			a.Kind, a.Signature = "incident", "grafana.datasource.down"
		}))
	req := memoryCreateRequest()
	req.Body.Kind = ptr("incident")
	req.Body.Signature = ptr("grafana.datasource.down")
	resp, err := memoryServer(store, memory).
		CreateMemoryAssertion(inArea("cx", domain.RoleAuthor), req)
	if err != nil {
		t.Fatalf("CreateMemoryAssertion: %v", err)
	}
	created := resp.(openapi.CreateMemoryAssertion200JSONResponse)
	if created.Kind != "incident" || created.Signature != "grafana.datasource.down" {
		t.Fatalf("identity = %q/%q, want the existing identity preserved", created.Kind, created.Signature)
	}
}

func TestCreateMemoryAssertion_explicitUnknownIdentity_isRefused(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	seedFinishedEvidence(t, store, "run-evidence", scope, domain.ScopeLabels(scope), "sha256:answer")
	req := memoryCreateRequest()
	req.Body.Kind = ptr("incident")
	req.Body.Signature = ptr("grafana.datasource.down")

	resp, err := memoryServer(store, memstore.NewMemory()).
		CreateMemoryAssertion(inArea("cx", domain.RoleAuthor), req)
	if err != nil {
		t.Fatalf("CreateMemoryAssertion: %v", err)
	}
	if _, bad := resp.(openapi.CreateMemoryAssertion400ApplicationProblemPlusJSONResponse); !bad {
		t.Fatalf("response = %T, want an unknown explicit identity refused", resp)
	}
}
