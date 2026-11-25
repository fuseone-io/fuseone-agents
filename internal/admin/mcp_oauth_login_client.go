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
	var (
		login  domain.MCPOAuthLogin
		secret string
	)
	// One snapshot for both rows: read apart, a rotation committed between
	// them would pair one version's client with another's secret, and send
	// the new secret to the old configuration's token endpoint.
	err := i.settings.ReadSnapshot(ctx, func(db settings.DB) error {
		row, err := i.settings.RevealOn(ctx, db, kind, at, settings.KindMCPServer, name)
		if errors.Is(err, settings.ErrNotFound) {
			return fmt.Errorf("%w: %s", ErrNoMCPOAuthLogin, name)
		}
		if err != nil {
			return err
		}
		var stored storedServer
		if err := json.Unmarshal(row.Value, &stored); err != nil {
			return fmt.Errorf("admin: decode MCP server %s: %w", name, err)
		}
		configured := stored.OAuthLogin.login()
		if configured == nil {
			return fmt.Errorf("%w: %s", ErrNoMCPOAuthLogin, name)
		}
		login = *configured
		if !login.HasClientSecret {
			return nil
		}
		sealed, err := i.settings.RevealOn(ctx, db, kind, at, settings.KindMCPOAuthLogin, name)
		if err != nil {
			return fmt.Errorf("admin: open the sign-in secret of %s: %w", name, err)
		}
		secret = sealed.Secret
		return nil
	})
	if err != nil {
		return domain.MCPOAuthLogin{}, "", err
	}
	return login, secret, nil
}
