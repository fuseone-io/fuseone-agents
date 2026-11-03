package guard

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrNoCallToken marks a call that brought no GitHub token in pass-through
// mode. The request is then never sent.
var ErrNoCallToken = errors.New("no GitHub token on this call")

type callTokenKey struct{}

func withCallToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, callTokenKey{}, token)
}

// RequestTokenSource is FACTORY_MCP_AUTH=github-app-token: the GitHub token
// of each request is the bearer of the call being served, which the platform
// minted for the calling role's App (#216). The server holds no credential.
type RequestTokenSource struct{}

// Token returns the call's bearer, or ErrNoCallToken.
func (RequestTokenSource) Token(ctx context.Context) (string, error) {
	if token, _ := ctx.Value(callTokenKey{}).(string); token != "" {
		return token, nil
	}
	return "", ErrNoCallToken
}

// bearer extracts a well-formed bearer from a header set, or "".
func bearer(h http.Header) string {
	token, ok := strings.CutPrefix(h.Get("Authorization"), "Bearer ")
	if !ok || token == "" || strings.ContainsAny(token, " \t\r\n") {
		return ""
	}
	return token
}

// callContext carries the call's own bearer into the tool's context. It is
// read from the headers of the HTTP request that carried this very call, not
// from the request that opened the session: a session outlives the hour an
// installation token lives, and the token used must be the one the identity
// check just accepted. Over stdio there are no headers, and the server's own
// App token source ignores the value.
func callContext(ctx context.Context, req *mcp.CallToolRequest) context.Context {
	if req == nil || req.Extra == nil {
		return ctx
	}
	if token := bearer(req.Extra.Header); token != "" {
		return withCallToken(ctx, token)
	}
	return ctx
}
