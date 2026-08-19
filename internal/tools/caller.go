package tools

import (
	"context"

	"github.com/fuseone/agents/internal/domain"
)

type callerKey struct{}
type invocationKey struct{}

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
// is one. Absence is a fact the transport must interpret with the tool
// server's credential model: discovery can be shared, while a concrete call to
// a user-only server must not fall back to the installation.
func CallerFrom(ctx context.Context) (domain.UserID, bool) {
	principal, ok := ctx.Value(callerKey{}).(domain.UserID)
	return principal, ok && principal != ""
}

// WithInvocation marks the context of a concrete tool call.
//
// Discovery and health checks also cross the MCP HTTP client, but they are not
// actions taken by an agent. A transport needs this bit to refuse user-only
// credentials for cron-triggered calls without breaking server discovery.
func WithInvocation(ctx context.Context) context.Context {
	return context.WithValue(ctx, invocationKey{}, true)
}

func IsInvocation(ctx context.Context) bool {
	ok, _ := ctx.Value(invocationKey{}).(bool)
	return ok
}
