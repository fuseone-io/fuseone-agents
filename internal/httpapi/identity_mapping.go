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
