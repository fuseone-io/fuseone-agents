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

/*
A policy is replaced whole, so a client that did not see what is stored erases
it. The headers reach the write: If-None-Match: * creates and is a 412 over a
policy that exists, If-Match edits and is a 412 over one that changed, and the
412 carries the policy's current ETag. The listing names each policy's ETag.
*/
func TestPutMCPEgressPolicy_preconditions_refuseWhatWouldEraseAPolicy(t *testing.T) {
	t.Parallel()
	fake := &fakeAdmin{}
	s := serverWith(t, fake)
	put := func(params openapi.PutMCPEgressPolicyParams, origins ...string) openapi.PutMCPEgressPolicyResponseObject {
		t.Helper()
		resp, err := s.PutMCPEgressPolicy(as(domain.RoleCurator), openapi.PutMCPEgressPolicyRequestObject{
			Params: params, Body: &openapi.MCPEgressPolicyInput{Company: "acme", Origins: origins},
		})
		if err != nil {
			t.Fatalf("put: %v", err)
		}
		return resp
	}
	created, ok := put(openapi.PutMCPEgressPolicyParams{IfNoneMatch: ptr("*")}, "https://jira.example.com").(openapi.PutMCPEgressPolicy204Response)
	if !ok || created.Headers.ETag == "" {
		t.Fatalf("a creation = %+v, want 204 with the policy's ETag", created)
	}

	again, ok := put(openapi.PutMCPEgressPolicyParams{IfNoneMatch: ptr("*")}, "https://crm.example.com").(openapi.PutMCPEgressPolicy412ApplicationProblemPlusJSONResponse)
	if !ok || valueOr(again.Headers.ETag) != created.Headers.ETag {
		t.Fatalf("a creation over it = %+v, want 412 carrying its ETag", again)
	}
	if _, ok := put(openapi.PutMCPEgressPolicyParams{IfMatch: ptr(`"stale"`)}, "https://crm.example.com").(openapi.PutMCPEgressPolicy412ApplicationProblemPlusJSONResponse); !ok {
		t.Fatal("an edit of another revision was not a 412")
	}
	if origins := fake.egress[domain.Scope{Company: "acme"}].Origins; len(origins) != 1 || origins[0] != "https://jira.example.com" {
		t.Fatalf("stored %v, want the policy as created", origins)
	}

	list, _ := s.ListMCPEgressPolicies(as(domain.RoleCurator), openapi.ListMCPEgressPoliciesRequestObject{})
	if got := list.(openapi.ListMCPEgressPolicies200JSONResponse).Policies[0].Etag; got != created.Headers.ETag {
		t.Fatalf("listed etag %q, want %q", got, created.Headers.ETag)
	}
	if _, ok := put(openapi.PutMCPEgressPolicyParams{IfMatch: ptr(created.Headers.ETag)}, "https://crm.example.com").(openapi.PutMCPEgressPolicy204Response); !ok {
		t.Fatal("an edit of the current revision was refused")
	}
}

func TestPutMCPEgressPolicy_bothPreconditionsOrAnUnreadableOne_isABadRequest(t *testing.T) {
	t.Parallel()
	s := serverWith(t, &fakeAdmin{})
	for name, params := range map[string]openapi.PutMCPEgressPolicyParams{
		"both":                {IfMatch: ptr(`"a"`), IfNoneMatch: ptr("*")},
		"an If-Match of junk": {IfMatch: ptr("junk")},
		"If-None-Match not *": {IfNoneMatch: ptr(`"a"`)},
	} {
		resp, err := s.PutMCPEgressPolicy(as(domain.RoleCurator), openapi.PutMCPEgressPolicyRequestObject{
			Params: params, Body: &openapi.MCPEgressPolicyInput{Company: "acme", Origins: []string{"https://jira.example.com"}},
		})
		if _, ok := resp.(openapi.PutMCPEgressPolicy400ApplicationProblemPlusJSONResponse); err != nil || !ok {
			t.Errorf("%s: = %T, %v; want 400", name, resp, err)
		}
	}
}

func TestDeleteMCPEgressPolicy_overAChangedPolicy_isAPreconditionFailure(t *testing.T) {
	t.Parallel()
	fake := &fakeAdmin{egress: map[domain.Scope]admin.MCPEgressPolicy{
		{Company: "acme"}: {Scope: domain.Scope{Company: "acme"}, Origins: []string{"https://wiki.example.com"}, Revision: `"now"`},
	}}
	s := serverWith(t, fake)
	resp, err := s.DeleteMCPEgressPolicy(as(domain.RoleCurator), openapi.DeleteMCPEgressPolicyRequestObject{
		Params: openapi.DeleteMCPEgressPolicyParams{Company: "acme", IfMatch: ptr(`"read"`)},
	})
	if _, ok := resp.(openapi.DeleteMCPEgressPolicy412ApplicationProblemPlusJSONResponse); err != nil || !ok || len(fake.egress) != 1 {
		t.Fatalf("= %T, %v, %d left; want 412 and the policy kept", resp, err, len(fake.egress))
	}
}
