package httpapi

import (
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
Only the platform's administration reads and writes the egress policies
(NT-019, decided 2026-11-13). An area's administrator configures that area,
and so is exactly whom the list holds: it can neither create nor widen it.
*/

var egressCX = domain.Scope{Company: "acme", Area: "cx"}

func TestMCPEgressPolicies_areThePlatformsAlone(t *testing.T) {
	t.Parallel()
	fake := &fakeAdmin{}
	s := serverWith(t, fake)
	areaAdmin := grantedAt(domain.RoleAdmin, egressCX)
	body := openapi.MCPEgressPolicyInput{Company: "acme", Area: ptr("cx"), Origins: []string{"https://jira.example.com"}}

	put, err := s.PutMCPEgressPolicy(areaAdmin, openapi.PutMCPEgressPolicyRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, ok := put.(openapi.PutMCPEgressPolicy403ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("put by the area's administrator = %T, want 403", put)
	}
	del, err := s.DeleteMCPEgressPolicy(areaAdmin, openapi.DeleteMCPEgressPolicyRequestObject{
		Params: openapi.DeleteMCPEgressPolicyParams{Company: "acme", Area: ptr("cx")},
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := del.(openapi.DeleteMCPEgressPolicy403ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("delete by the area's administrator = %T, want 403", del)
	}
	list, err := s.ListMCPEgressPolicies(areaAdmin, openapi.ListMCPEgressPoliciesRequestObject{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if _, ok := list.(openapi.ListMCPEgressPolicies403ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("list by the area's administrator = %T, want 403", list)
	}
	if len(fake.egress) != 0 {
		t.Errorf("stored %+v, want nothing from the area's administrator", fake.egress)
	}
}

func TestMCPEgressPolicies_thePlatformWritesAndReadsThem(t *testing.T) {
	t.Parallel()
	fake := &fakeAdmin{}
	s := serverWith(t, fake)
	body := openapi.MCPEgressPolicyInput{Company: "acme", Area: ptr("cx"), Origins: []string{"https://jira.example.com"}}

	put, err := s.PutMCPEgressPolicy(as(domain.RoleCurator), openapi.PutMCPEgressPolicyRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, ok := put.(openapi.PutMCPEgressPolicy204Response); !ok {
		t.Fatalf("put = %T, want 204", put)
	}
	list, err := s.ListMCPEgressPolicies(as(domain.RoleCurator), openapi.ListMCPEgressPoliciesRequestObject{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got, ok := list.(openapi.ListMCPEgressPolicies200JSONResponse)
	if !ok || len(got.Policies) != 1 || got.Policies[0].Company != "acme" || valueOr(got.Policies[0].Area) != "cx" {
		t.Errorf("list = %+v, want acme/cx's policy", list)
	}
	del, err := s.DeleteMCPEgressPolicy(as(domain.RoleCurator), openapi.DeleteMCPEgressPolicyRequestObject{
		Params: openapi.DeleteMCPEgressPolicyParams{Company: "acme", Area: ptr("cx")},
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := del.(openapi.DeleteMCPEgressPolicy204Response); !ok || len(fake.egress) != 0 {
		t.Errorf("delete = %T, stored %+v; want 204 and nothing left", del, fake.egress)
	}
}

func TestPutMCPEgressPolicy_anInvalidPolicy_isABadRequest(t *testing.T) {
	t.Parallel()
	fake := &fakeAdmin{err: admin.ErrEgressPolicyInvalid}
	s := serverWith(t, fake)
	body := openapi.MCPEgressPolicyInput{Company: "acme", Origins: []string{"https://*.example.com"}}
	put, err := s.PutMCPEgressPolicy(as(domain.RoleCurator), openapi.PutMCPEgressPolicyRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, ok := put.(openapi.PutMCPEgressPolicy400ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("put = %T, want 400", put)
	}
}
