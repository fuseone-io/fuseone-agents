package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// People is the directory the administration area reads and writes, declared
// here by the consumer.
type People interface {
	People(ctx context.Context) ([]domain.Person, error)
	SetGrants(ctx context.Context, principalID string, grants []domain.Grant, by string) error
}

// WithPeople wires the directory of who exists and what they hold.
func (s *Server) WithPeople(people People) *Server {
	s.people = people
	return s
}

func (s *Server) ListPeople(
	ctx context.Context, _ openapi.ListPeopleRequestObject,
) (openapi.ListPeopleResponseObject, error) {
	if err := auth.Require(ctx, domain.PermIdentityWrite, adminScope); err != nil {
		return openapi.ListPeople403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermIdentityWrite, adminScope),
		}, nil
	}
	if s.people == nil {
		return openapi.ListPeople200JSONResponse{Items: []openapi.Person{}}, nil
	}

	found, err := s.people.People(ctx)
	if err != nil {
		return nil, fmt.Errorf("list people: %w", err)
	}
	items := make([]openapi.Person, 0, len(found))
	for _, person := range found {
		items = append(items, toPerson(person))
	}
	return openapi.ListPeople200JSONResponse{Items: items}, nil
}

func (s *Server) SetGrants(
	ctx context.Context, req openapi.SetGrantsRequestObject,
) (openapi.SetGrantsResponseObject, error) {
	if err := auth.Require(ctx, domain.PermIdentityWrite, adminScope); err != nil {
		return openapi.SetGrants403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermIdentityWrite, adminScope),
		}, nil
	}
	if s.people == nil || req.Body == nil {
		return openapi.SetGrants204Response{}, nil
	}

	grants := make([]domain.Grant, 0, len(req.Body.Grants))
	for _, g := range req.Body.Grants {
		grants = append(grants, domain.Grant{
			Scope: domain.Scope{
				Company: domain.CompanyID(g.Company),
				Area:    domain.AreaID(g.Area),
			},
			Role: domain.Role(g.Role),
		})
	}

	// Recorded as granted by whoever is signing in to do it, which is what
	// keeps the asserted grants separable from the hand-made ones.
	if err := s.people.SetGrants(ctx, req.PrincipalId, grants, string(callerOf(ctx))); err != nil {
		return nil, fmt.Errorf("set grants for %s: %w", req.PrincipalId, err)
	}
	return openapi.SetGrants204Response{}, nil
}

func toPerson(p domain.Person) openapi.Person {
	out := openapi.Person{
		Id: p.ID, Kind: openapi.PersonKind(p.Kind),
		Display: p.Display, Disabled: p.Disabled,
	}
	out.Email = someString(p.Email)
	out.Provider = someString(p.Provider)
	out.Username = someString(p.Username)
	if !p.LastSeen.IsZero() && p.LastSeen.Year() > 1970 {
		out.LastSeen = &p.LastSeen
	}
	if len(p.Grants) > 0 {
		held := make([]openapi.HeldGrant, 0, len(p.Grants))
		for _, g := range p.Grants {
			held = append(held, openapi.HeldGrant{
				Company: string(g.Scope.Company), Area: string(g.Scope.Area),
				Role: openapi.HeldGrantRole(g.Role), Asserted: g.Asserted,
				By: someString(g.By),
			})
		}
		out.Grants = &held
	}
	return out
}
