package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/settings"
)

/*
The owner's ruling, and the ceiling's withdrawal (NT-019, phase 2 writers).

An area's curator classifies the tools of the area's own servers. The ruling
is stored at the scope where the tool's server is stored, and needs
tool:classify there, or at the platform's administration, which governs every
area. For a server of the installation the owner's row is the platform's, and
needs the platform's administration as it always did.

A tool whose server the caller may not classify, whether the platform's, a
sibling area's, or one nobody published, answers one refusal naming the
platform's administration: an area hears nothing about which it was.
*/

// classifyAsOwner is ClassifyTool with level: owner.
func (s *Server) classifyAsOwner(ctx context.Context, req openapi.ClassifyToolRequestObject) (openapi.ClassifyToolResponseObject, error) {
	if s.curator == nil {
		return nil, errNoAdministration
	}
	owner, refused, err := s.ownerClassifier(ctx, domain.ToolID(req.ToolId))
	if refused != nil || err != nil {
		return refused, err
	}
	ruling, refused, err := s.rulingFrom(ctx, req)
	if refused != nil || err != nil {
		return refused, err
	}
	err = s.curator.ClassifyAsOwner(ctx, owner, ruling)
	switch {
	case errors.Is(err, admin.ErrMCPServerUnknown):
		return openapi.ClassifyTool409ApplicationProblemPlusJSONResponse{
			Title:  "The tool's server changed",
			Detail: ptr("This tool's server is no longer stored where it was. Look again before ruling."),
			Status: http.StatusConflict,
		}, nil
	case err != nil:
		return openapi.ClassifyTool400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				notStored(err.Error())),
		}, nil
	}
	return openapi.ClassifyTool204Response{}, nil
}

/*
ownerClassifier finds the tool's server and authorizes the caller at its scope.

Refused before the lookup for whoever classifies nowhere. The owner it answers
is the exact row the authorization was for, and the write holds that row: a
server moved meanwhile does not receive the ruling.
*/
func (s *Server) ownerClassifier(
	ctx context.Context, tool domain.ToolID,
) (admin.ToolOwner, openapi.ClassifyToolResponseObject, error) {
	p, _ := auth.PrincipalFrom(ctx)
	outOfReach := openapi.ClassifyTool403ApplicationProblemPlusJSONResponse{
		ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermToolClassify, adminScope),
	}
	if len(p.ScopesFor(domain.PermToolClassify)) == 0 {
		return admin.ToolOwner{}, outOfReach, nil
	}
	owner, err := s.curator.OwnerOf(ctx, tool)
	switch {
	case errors.Is(err, admin.ErrNoOwnerScope) && p.Can(domain.PermToolClassify, adminScope):
		return admin.ToolOwner{}, openapi.ClassifyTool400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid("this tool's server is not stored at any scope, so it has no owner; rule it at the platform's level")),
		}, nil
	case errors.Is(err, admin.ErrNoOwnerScope):
		return admin.ToolOwner{}, outOfReach, nil
	case err != nil:
		return admin.ToolOwner{}, nil, fmt.Errorf("find the owner of %s: %w", tool, err)
	}
	if !mayClassify(p, owner.At.Kind, owner.At.Stored) {
		return admin.ToolOwner{}, outOfReach, nil
	}
	if owner.At.Kind == settings.ScopeInstallation {
		// The platform's row, recorded where the platform's rulings are.
		owner.At.Audit = adminScope
	}
	return owner, nil, nil
}

// mayClassify is mayConfigure for tool:classify: the permission at the
// server's scope, or at the platform's administration, and the
// installation's servers only from there.
func mayClassify(p domain.Principal, kind settings.ScopeKind, stored domain.Scope) bool {
	if p.Can(domain.PermToolClassify, adminScope) {
		return true
	}
	return kind != settings.ScopeInstallation && p.Can(domain.PermToolClassify, stored)
}

// WithdrawToolClassification removes the platform's ruling on a tool. Only
// the platform's administration does, and only the platform's row: the
// owner's, when there is one, is then in force.
func (s *Server) WithdrawToolClassification(
	ctx context.Context, req openapi.WithdrawToolClassificationRequestObject,
) (openapi.WithdrawToolClassificationResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermToolClassify); resp != nil {
		return openapi.WithdrawToolClassification403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.curator == nil {
		return nil, errNoAdministration
	}
	if req.Params.Level != openapi.WithdrawToolClassificationParamsLevelPlatform {
		return openapi.WithdrawToolClassification400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid("only the platform's ruling is withdrawn here")),
		}, nil
	}
	caller, _ := auth.PrincipalFrom(ctx)
	err := s.curator.Withdraw(ctx, caller.ID, adminScope, domain.ToolID(req.ToolId))
	switch {
	case errors.Is(err, admin.ErrRulingUnknown):
		return openapi.WithdrawToolClassification404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.ToolId),
		}, nil
	case err != nil:
		return nil, fmt.Errorf("withdraw the ruling on %s: %w", req.ToolId, err)
	}
	return openapi.WithdrawToolClassification204Response{}, nil
}

// toolRulings renders a tool's rulings for the listing, absent when there are none.
func toolRulings(in []domain.ToolRuling) *[]openapi.ToolRuling {
	if len(in) == 0 {
		return nil
	}
	out := make([]openapi.ToolRuling, 0, len(in))
	for _, r := range in {
		listed := openapi.ToolRuling{
			Level: openapi.RulingLevel(r.Level), Effect: openapi.Effect(r.Effect.String()),
			Untrusted: r.Untrusted, InForce: r.InForce,
			Scope: openapi.Scope{Company: string(r.Scope.Company), Area: string(r.Scope.Area)},
		}
		if r.Overridden {
			listed.Overridden = ptr(true)
		}
		if r.CompensatedBy != "" {
			listed.CompensatedBy = ptr(string(r.CompensatedBy))
		}
		if r.Dedupe.Enabled() {
			listed.Dedupe = &openapi.ToolDedupe{
				WindowSeconds: r.Dedupe.WindowSeconds,
				ArgPaths:      append([]string(nil), r.Dedupe.ArgPaths...),
			}
		}
		out = append(out, listed)
	}
	return &out
}
