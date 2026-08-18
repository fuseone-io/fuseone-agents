package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Branding is what this installation calls and paints itself.
type Branding interface {
	Current(ctx context.Context) (admin.BrandingConfig, error)
	Set(ctx context.Context, by domain.UserID, scope domain.Scope, config admin.BrandingConfig) error
}

func (s *Server) WithBranding(branding Branding) *Server {
	s.branding = branding
	return s
}

func (s *Server) GetBranding(
	ctx context.Context, _ openapi.GetBrandingRequestObject,
) (openapi.GetBrandingResponseObject, error) {
	config, err := s.brandingOrDefault(ctx)
	if err != nil {
		return nil, err
	}
	return openapi.GetBranding200JSONResponse(brandingOut(config)), nil
}

func (s *Server) GetAdminBranding(
	ctx context.Context, _ openapi.GetAdminBrandingRequestObject,
) (openapi.GetAdminBrandingResponseObject, error) {
	if refused := s.refuse(ctx, domain.PermToolRead); refused != nil {
		return openapi.GetAdminBranding403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *refused,
		}, nil
	}
	config, err := s.brandingOrDefault(ctx)
	if err != nil {
		return nil, err
	}
	return openapi.GetAdminBranding200JSONResponse(brandingOut(config)), nil
}

func (s *Server) SetAdminBranding(
	ctx context.Context, req openapi.SetAdminBrandingRequestObject,
) (openapi.SetAdminBrandingResponseObject, error) {
	if refused := s.mayWriteBrand(ctx); refused != nil {
		return openapi.SetAdminBranding403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *refused,
		}, nil
	}
	if s.branding == nil || req.Body == nil {
		return openapi.SetAdminBranding204Response{}, nil
	}

	if err := s.branding.Set(ctx, callerOf(ctx), adminScope, brandingIn(*req.Body)); err != nil {
		if errors.Is(err, admin.ErrBrandingInvalid) {
			return openapi.SetAdminBranding400ApplicationProblemPlusJSONResponse{
				BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
					notStored(err.Error())),
			}, nil
		}
		return nil, fmt.Errorf("set branding: %w", err)
	}
	return openapi.SetAdminBranding204Response{}, nil
}

func (s *Server) brandingOrDefault(ctx context.Context) (admin.BrandingConfig, error) {
	if s.branding == nil {
		return admin.DefaultBranding, nil
	}
	config, err := s.branding.Current(ctx)
	if err != nil {
		return admin.BrandingConfig{}, fmt.Errorf("read branding: %w", err)
	}
	return config, nil
}

func brandingOut(config admin.BrandingConfig) openapi.Branding {
	out := openapi.Branding{DisplayName: config.DisplayName}
	if config.LogoURL != "" {
		out.LogoUrl = ptr(config.LogoURL)
	}
	if config.IconURL != "" {
		out.IconUrl = ptr(config.IconURL)
	}
	if config.PrimaryColor != "" {
		out.PrimaryColor = ptr(config.PrimaryColor)
	}
	return out
}

func brandingIn(body openapi.Branding) admin.BrandingConfig {
	config := admin.BrandingConfig{DisplayName: body.DisplayName}
	if body.LogoUrl != nil {
		config.LogoURL = *body.LogoUrl
	}
	if body.IconUrl != nil {
		config.IconURL = *body.IconUrl
	}
	if body.PrimaryColor != nil {
		config.PrimaryColor = *body.PrimaryColor
	}
	return config
}

func (s *Server) mayWriteBrand(ctx context.Context) *openapi.ForbiddenApplicationProblemPlusJSONResponse {
	if err := auth.Require(ctx, domain.PermBrandWrite, adminScope); err != nil {
		body := forbidden(domain.PermBrandWrite, adminScope)
		return &body
	}
	return nil
}
