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

// SignInClient is a sign-in's client as it is now: the configuration, its
// secret, and the address of the server it is for, read together.
type SignInClient struct {
	Login     domain.MCPOAuthLogin
	Secret    string
	ServerURL string
}

/*
MCPOAuthLoginClient is the client a grant got through the server's browser
sign-in refreshes as: the server's configuration as it is now, its client
secret and its address, read at the scope the server is stored at, in one
snapshot. Read at each refresh rather than copied into every person's grant,
so a secret rotated on the server holds for everyone at once; read together,
so one version's client is never paired with another's secret or address.
*/
func (i *Integrations) MCPOAuthLoginClient(
	ctx context.Context, runtime domain.Scope, name string,
) (SignInClient, error) {
	kind, at := storedAt(runtime)
	var out SignInClient
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
		out.Login, out.ServerURL = *configured, stored.URL
		if !out.Login.HasClientSecret {
			return nil
		}
		sealed, err := i.settings.RevealOn(ctx, db, kind, at, settings.KindMCPOAuthLogin, name)
		if err != nil {
			return fmt.Errorf("admin: open the sign-in secret of %s: %w", name, err)
		}
		out.Secret = sealed.Secret
		return nil
	})
	if err != nil {
		return SignInClient{}, err
	}
	return out, nil
}
