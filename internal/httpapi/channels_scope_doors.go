package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/settings"
)

// errBindOutsideScope is a person bound on a company's or an area's connection
// who holds no grant inside it. Who they are is not said.
var errBindOutsideScope = errors.New("only a person holding a grant inside this connection's scope may be bound to it")

// conversationAuthority is where a conversation's doors are authorized
// (NT-019, phase 4 writers): an area's at the area, a company's at the
// company, a room for the whole installation at the installation, which is
// the platform's administration.
func conversationAuthority(scope domain.Scope) (settings.ScopeKind, domain.Scope) {
	switch {
	case scope.Company == domain.Installation:
		return settings.ScopeInstallation, domain.Scope{}
	case scope.Area != "":
		return settings.ScopeArea, scope
	default:
		return settings.ScopeCompany, scope
	}
}

// conversationConfigurer is configurerAt for a conversation of scope.
func (s *Server) conversationConfigurer(
	ctx context.Context, scope domain.Scope,
) (domain.UserID, *openapi.ForbiddenApplicationProblemPlusJSONResponse) {
	kind, stored := conversationAuthority(scope)
	return s.configurerAt(ctx, kind, stored)
}

/*
bindingAllowed judges a binding where its connection stands, under the
connection's lock (NT-019, phase 4 writers). On the installation's connection
binding stays the platform's, as before. On a company's or an area's, the
configurer there may bind, but only a person holding a grant inside that scope:
whoever owns the app can sign a click as any account bound to it, so the people
on it are kept to the people of its scope.
*/
func (s *Server) bindingAllowed(
	ctx context.Context, principal domain.UserID, refused **openapi.ForbiddenApplicationProblemPlusJSONResponse,
) func(settings.ScopeKind, domain.Scope) error {
	return func(kind settings.ScopeKind, stored domain.Scope) error {
		if _, *refused = s.configurerAt(ctx, kind, stored); *refused != nil {
			return errMCPServerRefused
		}
		if kind == settings.ScopeInstallation {
			return nil
		}
		inside, err := s.holdsGrantInside(ctx, principal, stored)
		if err != nil {
			return err
		}
		if !inside {
			return errBindOutsideScope
		}
		return nil
	}
}

// holdsGrantInside answers whether a person holds a grant whose scope is
// inside scope. A grant over it, at its company, is not inside it.
func (s *Server) holdsGrantInside(ctx context.Context, principal domain.UserID, scope domain.Scope) (bool, error) {
	if s.people == nil {
		return false, nil
	}
	found, err := s.people.People(ctx)
	if err != nil {
		return false, fmt.Errorf("list people for a binding: %w", err)
	}
	for _, person := range found {
		if person.ID != string(principal) || person.Disabled {
			continue
		}
		for _, held := range person.Grants {
			if scope.Contains(held.Scope) {
				return true, nil
			}
		}
	}
	return false, nil
}

// connectionRefused is configurerAt as an allow, recording the refusal.
func (s *Server) connectionRefused(
	ctx context.Context, refused **openapi.ForbiddenApplicationProblemPlusJSONResponse,
) func(settings.ScopeKind, domain.Scope) error {
	return func(kind settings.ScopeKind, stored domain.Scope) error {
		if _, *refused = s.configurerAt(ctx, kind, stored); *refused != nil {
			return errMCPServerRefused
		}
		return nil
	}
}

// errRoomOutsideConnection is a room whose connection, as the post read it,
// does not carry the room's scope.
var errRoomOutsideConnection = errors.New("the conversation is outside the scope of the connection it names")

// carriesRoom is the allow a test post opens its connection with: the
// connection's scope must contain the room's, judged on the row it posts
// through.
func carriesRoom(room domain.Scope) func(settings.ScopeKind, domain.Scope) error {
	return func(kind settings.ScopeKind, stored domain.Scope) error {
		serves, known := channel.ConnectionScope(kind, stored)
		if !known || !serves.Contains(room) {
			return errRoomOutsideConnection
		}
		return nil
	}
}
