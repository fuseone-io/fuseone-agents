package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
Who an agent's owner may name, and nothing else about them.

The screen that offers this is the agent's own, and its author holds no
authority over the directory. Pointing it at the administrative listing gave
them an empty control and a refusal nobody showed — and that listing answers
with the whole directory and its grants, which is why it is administrative.

So this answers one question, authorised by the act it belongs to: the right to
publish in the scope being asked about.
*/

// EligibleApprovers answers who may decide in a scope, by name.
type EligibleApprovers interface {
	ApproversNamed(ctx context.Context, scope domain.Scope) ([]auth.Eligible, error)
}

// WithEligibleApprovers wires the list an author picks from.
func (s *Server) WithEligibleApprovers(who EligibleApprovers) *Server {
	s.eligible = who
	return s
}

func (s *Server) ListEligibleApprovers(
	ctx context.Context, req openapi.ListEligibleApproversRequestObject,
) (openapi.ListEligibleApproversResponseObject, error) {
	scope := domain.Scope{}
	if req.Params.Company != nil {
		scope.Company = domain.CompanyID(*req.Params.Company)
	}
	if req.Params.Area != nil {
		scope.Area = domain.AreaID(*req.Params.Area)
	}

	// Checked in the scope asked about, like the agent listing beside it: an
	// author granted in cx must not read marketing by asking for it, and must
	// not be refused their own area for holding nothing at the installation.
	if err := auth.Require(ctx, domain.PermAgentPublish, scope); err != nil {
		return openapi.ListEligibleApprovers403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentPublish, scope),
		}, nil
	}
	if s.eligible == nil {
		return openapi.ListEligibleApprovers200JSONResponse{
			Items: []struct {
				Display string `json:"display"`
				Id      string `json:"id"`
			}{},
		}, nil
	}

	found, err := s.eligible.ApproversNamed(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("list eligible approvers: %w", err)
	}
	items := make([]struct {
		Display string `json:"display"`
		Id      string `json:"id"`
	}, 0, len(found))
	for _, one := range found {
		items = append(items, struct {
			Display string `json:"display"`
			Id      string `json:"id"`
		}{Display: one.Display, Id: string(one.ID)})
	}
	return openapi.ListEligibleApprovers200JSONResponse{Items: items}, nil
}
