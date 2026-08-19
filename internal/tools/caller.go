package tools

import (
	"context"

	"github.com/fuseone/agents/internal/domain"
)

type callerKey struct{}

// WithCaller records whose delegation a tool call is using.
//
// Discovery, health probes and local stdio sessions do not have such a human
// attached. A real call does: the runner carries OnBehalfOf from the run start
// to here so a remote transport can choose the user's own credential instead
// of silently acting as the installation.
func WithCaller(ctx context.Context, principal domain.UserID) context.Context {
	if principal == "" {
		return ctx
	}
	return context.WithValue(ctx, callerKey{}, principal)
}

// CallerFrom returns the human delegation attached to a tool call, when there
// is one. Absence means use the server's shared credential, not an anonymous
// user credential.
func CallerFrom(ctx context.Context) (domain.UserID, bool) {
	principal, ok := ctx.Value(callerKey{}).(domain.UserID)
	return principal, ok && principal != ""
}
