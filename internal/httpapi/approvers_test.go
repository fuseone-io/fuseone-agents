package httpapi

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
An author picks the people to tell, without authority over the directory.

The screen is the agent's own and its author holds `agent:publish` and nothing
about identity. Pointed at the administrative listing, it answered 403 and drew
an empty select with no error and no retry — a control that looks like an
installation with no approvers in it.
*/
func TestListEligibleApprovers_anAuthorInTheScope_getsTheList(t *testing.T) {
	t.Parallel()

	resp, err := NewServer(ledger.NewMemory(), "test").
		WithEligibleApprovers(namedApprovers{{ID: "usr_ana", Display: "Ana"}}).
		ListEligibleApprovers(inArea("cx", domain.RoleAuthor),
			openapi.ListEligibleApproversRequestObject{
				Params: openapi.ListEligibleApproversParams{
					Company: ptr("acme"), Area: ptr("cx"),
				},
			})
	if err != nil {
		t.Fatalf("ListEligibleApprovers: %v", err)
	}
	page, ok := resp.(openapi.ListEligibleApprovers200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the list", resp)
	}
	if len(page.Items) != 1 || page.Items[0].Id != "usr_ana" {
		t.Fatalf("items = %+v, want who may decide there", page.Items)
	}
	if page.Items[0].Display != "Ana" {
		t.Errorf("display = %q, want a name a person recognises", page.Items[0].Display)
	}
}

// And an author of another area does not read this one by asking for it. The
// list says who may decide in a scope, which is a fact about that scope.
func TestListEligibleApprovers_anotherAreasAuthor_isRefused(t *testing.T) {
	t.Parallel()

	resp, err := NewServer(ledger.NewMemory(), "test").
		WithEligibleApprovers(namedApprovers{{ID: "usr_ana", Display: "Ana"}}).
		ListEligibleApprovers(inArea("cx", domain.RoleAuthor),
			openapi.ListEligibleApproversRequestObject{
				Params: openapi.ListEligibleApproversParams{
					Company: ptr("acme"), Area: ptr("marketing"),
				},
			})
	if err != nil {
		t.Fatalf("ListEligibleApprovers: %v", err)
	}
	if _, ok := resp.(openapi.ListEligibleApprovers403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want forbidden", resp)
	}
}

type namedApprovers []auth.Eligible

func (n namedApprovers) DecidersNamed(
	context.Context, domain.Scope,
) ([]auth.Eligible, error) {
	return n, nil
}
