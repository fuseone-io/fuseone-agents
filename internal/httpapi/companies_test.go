package httpapi

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
Who may create a company.

The whole point of the scope above every company: held inside one, this
authority would let acme's administrator mint another company and grant
themselves in it, which is not a tightening anybody would notice. So the check
is against the installation and a company grant does not carry it, however
senior the role.
*/
func TestCreateCompany_curatorOfOneCompany_isRefused(t *testing.T) {
	t.Parallel()
	spy := &companySpy{}
	s := NewServer(ledger.NewMemory(), "test").WithCompanies(spy)

	resp, err := s.CreateCompany(inCompany("acme", domain.RoleCurator),
		openapi.CreateCompanyRequestObject{
			Body: &openapi.CreateCompanyJSONRequestBody{Id: "outra"},
		})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}

	if _, refused := resp.(openapi.CreateCompany403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want a refusal", resp)
	}
	if spy.created != 0 {
		t.Error("a company was created by somebody who governs one company")
	}
}

func TestCreateCompany_heldOverTheInstallation_isAllowed(t *testing.T) {
	t.Parallel()
	spy := &companySpy{}
	s := NewServer(ledger.NewMemory(), "test").WithCompanies(spy)

	resp, err := s.CreateCompany(everywhere(domain.RoleCurator),
		openapi.CreateCompanyRequestObject{
			Body: &openapi.CreateCompanyJSONRequestBody{Id: "outra", Label: ptr("Outra")},
		})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	if _, ok := resp.(openapi.CreateCompany201JSONResponse); !ok {
		t.Fatalf("response = %T, want the company", resp)
	}
	if spy.created != 1 {
		t.Error("nothing was created")
	}
}

// Renaming and withdrawing are two decisions, so they are two entries in the
// trail. A single write would leave an auditor inferring which happened.
func TestUpdateCompany_renamesAndWithdraws_recordsBoth(t *testing.T) {
	t.Parallel()
	spy := &companySpy{}
	s := NewServer(ledger.NewMemory(), "test").WithCompanies(spy)

	if _, err := s.UpdateCompany(everywhere(domain.RoleCurator),
		openapi.UpdateCompanyRequestObject{
			Company: "acme",
			Body: &openapi.UpdateCompanyJSONRequestBody{
				Label: ptr("Acme SA"), Archived: ptr(true),
			},
		}); err != nil {
		t.Fatalf("UpdateCompany: %v", err)
	}

	if spy.renamed != 1 || spy.archived != 1 {
		t.Errorf("renamed=%d archived=%d, want one of each", spy.renamed, spy.archived)
	}
}

func TestListCompanies_withoutTheAuthority_isRefused(t *testing.T) {
	t.Parallel()
	s := NewServer(ledger.NewMemory(), "test").WithCompanies(&companySpy{})

	resp, err := s.ListCompanies(inCompany("acme", domain.RoleCurator),
		openapi.ListCompaniesRequestObject{})
	if err != nil {
		t.Fatalf("ListCompanies: %v", err)
	}
	if _, refused := resp.(openapi.ListCompanies403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want a refusal", resp)
	}
}

// --- callers ----------------------------------------------------------------

func inCompany(company string, role domain.Role) context.Context {
	return auth.WithPrincipal(context.Background(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{
			Scope: domain.Scope{Company: domain.CompanyID(company)}, Role: role,
		}},
	})
}

func everywhere(role domain.Role) context.Context {
	return auth.WithPrincipal(context.Background(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{
			Scope: domain.Scope{Company: domain.Installation}, Role: role,
		}},
	})
}

type companySpy struct{ created, renamed, archived int }

func (c *companySpy) List(context.Context) ([]admin.Company, error) { return nil, nil }

func (c *companySpy) Create(
	_ context.Context, id domain.CompanyID, label string, _ domain.UserID,
) (admin.Company, error) {
	c.created++
	return admin.Company{ID: id, Label: label}, nil
}

func (c *companySpy) Rename(context.Context, domain.CompanyID, string, domain.UserID) error {
	c.renamed++
	return nil
}

func (c *companySpy) Archive(context.Context, domain.CompanyID, domain.UserID) error {
	c.archived++
	return nil
}

func (c *companySpy) Restore(context.Context, domain.CompanyID, domain.UserID) error {
	return nil
}
