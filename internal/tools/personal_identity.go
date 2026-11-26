package tools

import (
	"context"
	"errors"
)

/*
ErrSessionIdentityChanged is a request refused, before it was sent, because the
person's credential no longer speaks as the identity their session was opened
as. A server that fixes who is calling at initialize would run anything sent
on that session as the old identity, so nothing is: the pool retires the
session and the call runs again on one opened as the new identity.
*/
var ErrSessionIdentityChanged = errors.New("tools: the person's credential no longer speaks as the identity their session was opened as")

// identityTries is how many sessions one call may go through while the
// person's credential keeps moving under it.
const identityTries = 3

type sessionIdentityKey struct{}

// WithSessionIdentity marks a request as one made on a session opened as
// identity (see domain.MCPCredentials.SessionIdentity).
func WithSessionIdentity(ctx context.Context, identity string) context.Context {
	return context.WithValue(ctx, sessionIdentityKey{}, identity)
}

// SessionIdentityFrom is the identity the request's session was opened as,
// when it is a person's session.
func SessionIdentityFrom(ctx context.Context) (string, bool) {
	identity, ok := ctx.Value(sessionIdentityKey{}).(string)
	return identity, ok
}
