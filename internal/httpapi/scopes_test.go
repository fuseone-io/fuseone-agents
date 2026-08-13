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
Declaring an area, in a company the caller can actually see.

The permission was checked against a fixed administration scope rather than
against the company being written, so an operator could register an area in any
company at all — and then not see it, because listing is filtered by the scopes
they hold. The console reported success, the row existed, and the screen stayed
empty. A write nobody can read back is worse than a refusal: it leaves rows in
a governance table that no grant reaches.
*/
func TestRegisterScope_companyTheCallerDoesNotReach_isRefusedAndNotWritten(t *testing.T) {
	t.Parallel()
	areas := &areaSpy{}
	s := NewServer(ledger.NewMemory(), "test").WithAreas(areas)

	resp, err := s.RegisterScope(as(domain.RoleCurator), openapi.RegisterScopeRequestObject{
		Body: &openapi.RegisterScopeJSONRequestBody{Company: "outra-empresa", Name: "vendas"},
	})
	if err != nil {
		t.Fatalf("RegisterScope: %v", err)
	}

	if _, refused := resp.(openapi.RegisterScope403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want a refusal naming the permission", resp)
	}
	if areas.written != 0 {
		t.Errorf("wrote %d rows for a company the caller cannot reach", areas.written)
	}
}

func TestRegisterScope_companyTheCallerHolds_isWritten(t *testing.T) {
	t.Parallel()
	areas := &areaSpy{}
	s := NewServer(ledger.NewMemory(), "test").WithAreas(areas)

	resp, err := s.RegisterScope(acrossCompany(domain.RoleCurator), openapi.RegisterScopeRequestObject{
		Body: &openapi.RegisterScopeJSONRequestBody{
			Company: string(adminScope.Company), Name: "Risco de Crédito",
		},
	})
	if err != nil {
		t.Fatalf("RegisterScope: %v", err)
	}
	if _, ok := resp.(openapi.RegisterScope200JSONResponse); !ok {
		t.Fatalf("response = %T, want the registered scope", resp)
	}
	if areas.written != 1 {
		t.Errorf("wrote %d rows", areas.written)
	}
}

// A label is what a person reads; the name is what the platform stores. Both
// travelling was the other half of what looked like a form that did nothing.
func TestRegisterScope_labelSupplied_reachesTheStoreBesideTheName(t *testing.T) {
	t.Parallel()
	areas := &areaSpy{}
	s := NewServer(ledger.NewMemory(), "test").WithAreas(areas)

	shown := "Risco de Crédito"
	if _, err := s.RegisterScope(acrossCompany(domain.RoleCurator), openapi.RegisterScopeRequestObject{
		Body: &openapi.RegisterScopeJSONRequestBody{
			Company: string(adminScope.Company), Name: "risco", Label: &shown,
		},
	}); err != nil {
		t.Fatalf("RegisterScope: %v", err)
	}

	if areas.typed != "risco" || areas.label != shown {
		t.Errorf("stored name=%q label=%q, want the name folded and the label kept",
			areas.typed, areas.label)
	}
}

// A grant over the whole company, which is what declaring an area needs.
//
// Deliberately not a grant on one area: inventing the areas a company has is a
// company-level act, and somebody trusted inside `default/platform` has no
// business creating `default/juridico` alongside it.
func acrossCompany(role domain.Role) context.Context {
	return auth.WithPrincipal(context.Background(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: adminScope.Company}, Role: role}},
	})
}

type areaSpy struct {
	written      int
	typed, label string
	company      domain.CompanyID
}

func (a *areaSpy) List(context.Context, []domain.Scope) ([]domain.RegisteredScope, error) {
	return nil, nil
}

func (a *areaSpy) Put(
	_ context.Context, company domain.CompanyID, typed, label string, _ domain.UserID,
) (domain.RegisteredScope, error) {
	a.written++
	a.company, a.typed, a.label = company, typed, label
	return domain.RegisteredScope{
		Scope: domain.Scope{Company: company, Area: "risco"}, Label: label,
	}, nil
}

func (a *areaSpy) Delete(context.Context, domain.CompanyID, domain.AreaID) error { return nil }
