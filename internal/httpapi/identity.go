package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Identity is the administration store for how people sign in, declared here
// by the consumer.
type Identity interface {
	IdentityProviders(ctx context.Context) ([]domain.IdentityProvider, error)
	PutIdentityProvider(ctx context.Context, by domain.UserID, scope domain.Scope,
		provider domain.IdentityProvider, clientSecret string) error
	DeleteIdentityProvider(ctx context.Context, by domain.UserID, scope domain.Scope, id string) error
	IdentitySecret(ctx context.Context, id string) (string, error)
}

// SignIn is the live registry the sign-in routes read from.
//
// Written here, on save, rather than only at start-up. Discovery is the one
// part of this that can fail, and failing it in front of the operator who
// typed the address beats failing it later in front of somebody trying to sign
// in — who cannot fix it and may not even know what broke.
type SignIn interface {
	Add(ctx context.Context, p *auth.OIDCProvider) error
	Remove(id string)
}

// WithIdentity wires the administration of sign-in.
func (s *Server) WithIdentity(store Identity, live SignIn) *Server {
	s.identity, s.signIn = store, live
	return s
}

func (s *Server) ListIdentityProviders(
	ctx context.Context, _ openapi.ListIdentityProvidersRequestObject,
) (openapi.ListIdentityProvidersResponseObject, error) {
	if err := auth.Require(ctx, domain.PermIdentityWrite, adminScope); err != nil {
		return openapi.ListIdentityProviders403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermIdentityWrite, adminScope),
		}, nil
	}
	if s.identity == nil {
		return openapi.ListIdentityProviders200JSONResponse{Items: []openapi.IdentityProvider{}}, nil
	}

	found, err := s.identity.IdentityProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list identity providers: %w", err)
	}
	items := make([]openapi.IdentityProvider, 0, len(found))
	for _, p := range found {
		items = append(items, toIdentityProvider(p))
	}
	return openapi.ListIdentityProviders200JSONResponse{Items: items}, nil
}

func (s *Server) PutIdentityProvider(
	ctx context.Context, req openapi.PutIdentityProviderRequestObject,
) (openapi.PutIdentityProviderResponseObject, error) {
	if err := auth.Require(ctx, domain.PermIdentityWrite, adminScope); err != nil {
		return openapi.PutIdentityProvider403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermIdentityWrite, adminScope),
		}, nil
	}
	if s.identity == nil || req.Body == nil {
		return openapi.PutIdentityProvider204Response{}, nil
	}

	provider := fromIdentityProviderInput(req.Id, *req.Body)
	secret := ""
	if req.Body.ClientSecret != nil {
		secret = *req.Body.ClientSecret
	}
	if err := s.identity.PutIdentityProvider(ctx, callerOf(ctx), adminScope, provider, secret); err != nil {
		return openapi.PutIdentityProvider400ApplicationProblemPlusJSONResponse(
			upstreamRefused(err.Error())), nil
	}

	if err := s.register(ctx, provider, secret); err != nil {
		// Kept rather than discarded: an installation is routinely configured
		// before its identity provider is reachable from this network, and
		// throwing away everything somebody typed because a host was down for
		// a minute is not a kindness. Start-up discovers it again.
		//
		// The message says both halves. "Could not be reached" alone would
		// read as nothing having been saved, beside a row that just appeared.
		return openapi.PutIdentityProvider400ApplicationProblemPlusJSONResponse(
			savedNotReachable(err.Error())), nil
	}
	return openapi.PutIdentityProvider204Response{}, nil
}

func (s *Server) DeleteIdentityProvider(
	ctx context.Context, req openapi.DeleteIdentityProviderRequestObject,
) (openapi.DeleteIdentityProviderResponseObject, error) {
	if err := auth.Require(ctx, domain.PermIdentityWrite, adminScope); err != nil {
		return openapi.DeleteIdentityProvider403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermIdentityWrite, adminScope),
		}, nil
	}
	if s.identity == nil {
		return openapi.DeleteIdentityProvider204Response{}, nil
	}

	if err := s.identity.DeleteIdentityProvider(ctx, callerOf(ctx), adminScope, req.Id); err != nil {
		return nil, fmt.Errorf("delete identity provider %s: %w", req.Id, err)
	}
	if s.signIn != nil {
		// Out of the live registry too, or it keeps accepting sign-ins for a
		// configuration nobody can see any more.
		s.signIn.Remove(req.Id)
	}
	return openapi.DeleteIdentityProvider204Response{}, nil
}

// register puts the provider into the live registry, discovering it on the
// way. The secret comes from the vault when the caller did not send one, so
// saving a mapping change never silently registers a provider with no
// credential.
func (s *Server) register(ctx context.Context, provider domain.IdentityProvider, secret string) error {
	if s.signIn == nil {
		return nil
	}
	if secret == "" {
		stored, err := s.identity.IdentitySecret(ctx, provider.ID)
		if err != nil {
			return fmt.Errorf("read stored client secret: %w", err)
		}
		secret = stored
	}
	if !provider.Enabled {
		// Disabled is a way of switching a provider off without losing its
		// configuration, so it leaves the registry rather than joining it.
		s.signIn.Remove(provider.ID)
		return nil
	}

	return s.signIn.Add(ctx, &auth.OIDCProvider{
		ID: provider.ID, Display: provider.Display, Issuer: provider.Issuer,
		ClientID: provider.ClientID, Revision: auth.IdentityProviderRevision(provider), ClientSecret: secret,
		GroupsClaim: provider.GroupsClaim, Mappings: provider.Mappings,
	})
}
