package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

// ErrNoMCPOAuthLogin is a server people no longer sign in to: its sign-in was
// removed, or it moved, since the grant was got.
var ErrNoMCPOAuthLogin = errors.New("admin: the server has no browser sign-in")

/*
MCPOAuthLoginClient is the client a grant got through the server's browser
sign-in refreshes as: the server's configuration as it is now, and its
client secret, read at the scope the server is stored at. Read at each
refresh rather than copied into every person's grant, so a secret rotated on
the server holds for everyone at once.
*/
func (i *Integrations) MCPOAuthLoginClient(
	ctx context.Context, runtime domain.Scope, name string,
) (domain.MCPOAuthLogin, string, error) {
	kind, at := storedAt(runtime)
	row, err := i.settings.Get(ctx, kind, at, settings.KindMCPServer, name)
	if errors.Is(err, settings.ErrNotFound) {
		return domain.MCPOAuthLogin{}, "", fmt.Errorf("%w: %s", ErrNoMCPOAuthLogin, name)
	}
	if err != nil {
		return domain.MCPOAuthLogin{}, "", err
	}
	var stored storedServer
	if err := json.Unmarshal(row.Value, &stored); err != nil {
		return domain.MCPOAuthLogin{}, "", fmt.Errorf("admin: decode MCP server %s: %w", name, err)
	}
	login := stored.OAuthLogin.login()
	if login == nil {
		return domain.MCPOAuthLogin{}, "", fmt.Errorf("%w: %s", ErrNoMCPOAuthLogin, name)
	}
	if !login.HasClientSecret {
		return *login, "", nil
	}
	sealed, err := i.settings.Reveal(ctx, kind, at, settings.KindMCPOAuthLogin, name)
	if err != nil {
		return domain.MCPOAuthLogin{}, "", fmt.Errorf("admin: open the sign-in secret of %s: %w", name, err)
	}
	return *login, sealed.Secret, nil
}
