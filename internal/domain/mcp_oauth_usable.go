package domain

import "errors"

// ErrMCPOAuthUnusable is an OAuth grant that holds something but nothing a
// request can carry: no access token, and no refresh token with a token
// endpoint to renew one at.
var ErrMCPOAuthUnusable = errors.New(
	"mcp oauth: the grant has no access token and no refresh token with a token URL, so " +
		"nothing could be sent with it — give a token, or remove the grant")

/*
CheckOAuthUsable refuses a grant a request could never carry.

The rule the worker applies before it builds a server's client, applied where
the grant is written: stored, such a grant replaced one that worked and the
server stopped answering later, on a timer, with nobody at the screen. An empty
grant is a removal and nil is silence; neither is refused.
*/
func CheckOAuthUsable(grant *MCPOAuthGrant) error {
	if grant == nil || grant.Empty() || grant.AccessToken != "" || grant.CanRefresh() {
		return nil
	}
	return ErrMCPOAuthUnusable
}
