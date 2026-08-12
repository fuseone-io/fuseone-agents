package httpapi

import (
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

func toIdentityProvider(p domain.IdentityProvider) openapi.IdentityProvider {
	out := openapi.IdentityProvider{
		Id: p.ID, Display: p.Display, Issuer: p.Issuer,
		HasSecret: p.HasSecret, Enabled: p.Enabled,
	}
	out.ClientId = someString(p.ClientID)
	out.GroupsClaim = someString(p.GroupsClaim)
	out.UpdatedBy = someString(p.UpdatedBy)
	if !p.UpdatedAt.IsZero() {
		out.UpdatedAt = &p.UpdatedAt
	}
	if len(p.Mappings) > 0 {
		mapped := make([]openapi.GroupMapping, 0, len(p.Mappings))
		for _, m := range p.Mappings {
			mapped = append(mapped, openapi.GroupMapping{
				Group: m.Group, Company: m.Company, Area: m.Area,
				Role: openapi.GroupMappingRole(m.Role),
			})
		}
		out.Mappings = &mapped
	}
	return out
}

func fromIdentityProviderInput(id string, in openapi.IdentityProviderInput) domain.IdentityProvider {
	out := domain.IdentityProvider{
		ID: id, Display: in.Display, Issuer: in.Issuer, ClientID: in.ClientId,
		// Absent means on. A provider configured and then not switched on is
		// almost always a provider somebody meant to use.
		Enabled: in.Enabled == nil || *in.Enabled,
	}
	if in.GroupsClaim != nil {
		out.GroupsClaim = *in.GroupsClaim
	}
	if in.Mappings != nil {
		for _, m := range *in.Mappings {
			out.Mappings = append(out.Mappings, domain.GroupMapping{
				Group: m.Group, Company: m.Company, Area: m.Area, Role: string(m.Role),
			})
		}
	}
	return out
}

func toRegressionCase(c domain.RegressionCase) openapi.RegressionCase {
	out := openapi.RegressionCase{
		Id:           c.ID,
		Expectations: toExpectations(c.Expectations),
		FromRun:      someString(string(c.FromRun)),
		Note:         someString(c.Note),
		CreatedBy:    someString(string(c.CreatedBy)),
	}
	if !c.CreatedAt.IsZero() {
		out.CreatedAt = &c.CreatedAt
	}
	return out
}

func toExpectations(in []domain.Expectation) []openapi.Expectation {
	out := make([]openapi.Expectation, 0, len(in))
	for _, e := range in {
		out = append(out, openapi.Expectation{
			Kind:  openapi.ExpectationKind(e.Kind),
			Step:  someString(e.Step),
			Value: someString(e.Value),
		})
	}
	return out
}

func fromExpectations(in []openapi.Expectation) []domain.Expectation {
	out := make([]domain.Expectation, 0, len(in))
	for _, e := range in {
		expectation := domain.Expectation{Kind: domain.ExpectationKind(e.Kind)}
		if e.Step != nil {
			expectation.Step = *e.Step
		}
		if e.Value != nil {
			expectation.Value = *e.Value
		}
		out = append(out, expectation)
	}
	return out
}
