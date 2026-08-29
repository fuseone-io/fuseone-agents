package httpapi

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	memstore "github.com/fuseone/agents/internal/memory"
)

func TestCreateMemoryAssertion_withoutLegacyIdentity_derivesItFromTheSubject(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	seedFinishedEvidence(t, store, "run-evidence", scope, domain.ScopeLabels(scope), "sha256:answer")
	req := memoryCreateRequest()
	req.Body.Kind = nil
	req.Body.Signature = nil
	req.Body.Subject = "  Grafana datasource timeout  "

	resp, err := memoryServer(store, memstore.NewMemory()).
		CreateMemoryAssertion(inArea("cx", domain.RoleAuthor), req)
	if err != nil {
		t.Fatalf("CreateMemoryAssertion: %v", err)
	}
	created := resp.(openapi.CreateMemoryAssertion200JSONResponse)
	if created.Kind != domain.MemoryKindFact || created.Signature != "Grafana datasource timeout" {
		t.Fatalf("identity = %q/%q, want fact and the trimmed subject", created.Kind, created.Signature)
	}
}

func TestCreateMemoryAssertion_partialLegacyIdentity_isRefused(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	seedFinishedEvidence(t, store, "run-evidence", scope, domain.ScopeLabels(scope), "sha256:answer")
	req := memoryCreateRequest()
	req.Body.Kind = ptr("incident")
	req.Body.Signature = nil

	resp, err := memoryServer(store, memstore.NewMemory()).
		CreateMemoryAssertion(inArea("cx", domain.RoleAuthor), req)
	if err != nil {
		t.Fatalf("CreateMemoryAssertion: %v", err)
	}
	if _, bad := resp.(openapi.CreateMemoryAssertion400ApplicationProblemPlusJSONResponse); !bad {
		t.Fatalf("response = %T, want 400", resp)
	}
}

func TestCreateMemoryAssertion_derivedIdentityStillPassesTheSecretPolicy(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	seedFinishedEvidence(t, store, "run-evidence", scope, domain.ScopeLabels(scope), "sha256:answer")
	req := memoryCreateRequest()
	req.Body.Kind = nil
	req.Body.Signature = nil
	req.Body.Subject = "-----BEGIN RSA PRIVATE KEY-----"

	resp, err := memoryServer(store, memstore.NewMemory()).
		CreateMemoryAssertion(inArea("cx", domain.RoleAuthor), req)
	if err != nil {
		t.Fatalf("CreateMemoryAssertion: %v", err)
	}
	bad, ok := resp.(openapi.CreateMemoryAssertion400ApplicationProblemPlusJSONResponse)
	if !ok || bad.Type == nil || *bad.Type != string(CodeMemorySecret) {
		t.Fatalf("response = %#v, want the final derived assertion refused as a secret", resp)
	}
}
