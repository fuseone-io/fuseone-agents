package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
Reading one configured thing, by the key it is stored under.

The listings read a kind whole, which is right for a screen showing all of it.
A declarative client refreshing N resources makes N of these, so each has to
cost the same however many others exist.
*/

// ErrProviderUnknown means nobody configured a provider under that name.
var ErrProviderUnknown = errors.New("admin: no such provider")

/*
MCPServerNamed reads one server configured here, with what may be said about its
sealed document.

That document is opened here and goes no further: only the summary and the
fingerprint leave this function. Worked out on every read rather than copied out
on write, so nothing leaves the seal, and rows stored before this existed need no
migration.

A document that will not open is an error. Reported as absent, a client would
read the credential as gone and write it back.
*/
func (i *Integrations) MCPServerNamed(ctx context.Context, name string) (domain.MCPServer, error) {
	return i.MCPServerNamedIf(ctx, name, nil)
}

// MCPServerNamedIf is MCPServerNamed that asks allow, with the server's
// stored scope, before its sealed document is opened (NT-019): a caller
// refused the server never reaches the vault.
func (i *Integrations) MCPServerNamedIf(
	ctx context.Context, name string, allow func(settings.ScopeKind, domain.Scope) error,
) (domain.MCPServer, error) {
	// Found and opened in one statement: the answer is the state the read
	// began on, at whatever scope the server is stored (NT-019).
	row, err := i.settings.RevealNamedIf(ctx, settings.KindMCPServer, name, allow)
	if errors.Is(err, settings.ErrNotFound) {
		return domain.MCPServer{}, fmt.Errorf("%w: %s", ErrMCPServerUnknown, name)
	}
	if err != nil {
		return domain.MCPServer{}, fmt.Errorf("admin: read MCP server %s: %w", name, err)
	}
	server, err := serverFromRow(row)
	if err != nil {
		return domain.MCPServer{}, err
	}
	creds := domain.ReadMCPCredentials(row.Secret)
	summary := creds.Summary()
	server.Credential = &summary
	server.SecretFingerprint = i.settings.Fingerprint(row, creds.FingerprintMaterial())
	server.Revision = i.mcpRevision(row)
	return server, nil
}

// ProviderNamed reads one provider, naming its key without saying what it is.
// The key is opened here and goes no further.
func (i *Integrations) ProviderNamed(ctx context.Context, name string) (domain.ModelProvider, error) {
	row, err := i.settings.Reveal(ctx, settings.ScopeInstallation, domain.Scope{}, settings.KindModelProvider, name)
	if errors.Is(err, settings.ErrNotFound) {
		return domain.ModelProvider{}, fmt.Errorf("%w: %s", ErrProviderUnknown, name)
	}
	if err != nil {
		return domain.ModelProvider{}, fmt.Errorf("admin: read provider %s: %w", name, err)
	}
	provider, err := providerFromRow(row)
	if err != nil {
		return domain.ModelProvider{}, err
	}
	provider.SecretFingerprint = i.settings.Fingerprint(row, []byte(row.Secret))
	provider.Revision = i.providerRevised(domain.IfMatch{}).of(row)
	return provider, nil
}
