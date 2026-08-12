package httpapi

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

type fakePeople struct {
	listed []domain.Person
	set    []domain.Grant
	by     string
}

func (f *fakePeople) People(context.Context) ([]domain.Person, error) { return f.listed, nil }

func (f *fakePeople) SetGrants(_ context.Context, _ string, grants []domain.Grant, by string) error {
	f.set, f.by = grants, by
	return nil
}

func peopleServer(p *fakePeople) *Server {
	return NewServer(ledger.NewMemory(), "test").WithPeople(p)
}

func TestListPeople_marksWhatAnOperatorCannotRevokeHere(t *testing.T) {
	t.Parallel()

	people := &fakePeople{listed: []domain.Person{{
		ID: "usr_ana", Kind: domain.PrincipalUser, Display: "Ana", Provider: "keycloak",
		Grants: []domain.HeldGrant{
			{Grant: domain.Grant{Scope: domain.Scope{Company: "acme", Area: "cx"}, Role: domain.RoleAuthor},
				Asserted: true, By: "oidc:keycloak"},
			{Grant: domain.Grant{Scope: domain.Scope{Company: "acme", Area: "cx"}, Role: domain.RoleApprover},
				By: "usr_operator"},
		},
	}}}

	resp, err := peopleServer(people).ListPeople(as(domain.RoleCurator), openapi.ListPeopleRequestObject{})
	if err != nil {
		t.Fatalf("ListPeople: %v", err)
	}
	got := resp.(openapi.ListPeople200JSONResponse)
	held := *got.Items[0].Grants
	// Revoking an asserted grant here would last until its holder signs in
	// again, so the screen has to be able to tell them apart.
	if !held[0].Asserted || held[1].Asserted {
		t.Errorf("grants = %+v", held)
	}
}

func TestListPeople_includesSomebodyWithNoGrantAtAll(t *testing.T) {
	t.Parallel()

	people := &fakePeople{listed: []domain.Person{{
		ID: "usr_novo", Kind: domain.PrincipalUser, Display: "Novo",
	}}}

	resp, err := peopleServer(people).ListPeople(as(domain.RoleCurator), openapi.ListPeopleRequestObject{})
	if err != nil {
		t.Fatalf("ListPeople: %v", err)
	}
	got := resp.(openapi.ListPeople200JSONResponse)
	// They can sign in and do nothing, which is exactly who an operator opens
	// this screen looking for.
	if len(got.Items) != 1 || got.Items[0].Grants != nil {
		t.Errorf("items = %+v", got.Items)
	}
}

func TestSetGrants_recordsWhoGranted(t *testing.T) {
	t.Parallel()

	people := &fakePeople{}
	_, err := peopleServer(people).SetGrants(as(domain.RoleCurator), openapi.SetGrantsRequestObject{
		PrincipalId: "usr_ana",
		Body: &openapi.SetGrantsJSONRequestBody{
			Grants: []openapi.GrantInput{{Company: "acme", Area: "cx", Role: "approver"}},
		},
	})
	if err != nil {
		t.Fatalf("SetGrants: %v", err)
	}

	if len(people.set) != 1 || people.set[0].Role != domain.RoleApprover {
		t.Fatalf("grants = %+v", people.set)
	}
	// Whoever signed in to do it. That is what keeps a hand-made grant
	// separable from one an identity provider asserts.
	if people.by == "" {
		t.Error("the grant records nobody as having made it")
	}
}

func TestSetGrants_withoutIdentityWrite_isForbidden(t *testing.T) {
	t.Parallel()

	people := &fakePeople{}
	resp, err := peopleServer(people).SetGrants(as(domain.RoleAuthor), openapi.SetGrantsRequestObject{
		PrincipalId: "usr_ana",
		Body: &openapi.SetGrantsJSONRequestBody{
			Grants: []openapi.GrantInput{{Company: "acme", Area: "cx", Role: "curator"}},
		},
	})
	if err != nil {
		t.Fatalf("SetGrants: %v", err)
	}
	// Granting yourself curator is the one escalation this must not allow.
	if _, ok := resp.(openapi.SetGrants403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want it refused", resp)
	}
	if len(people.set) != 0 {
		t.Error("it was granted anyway")
	}
}
