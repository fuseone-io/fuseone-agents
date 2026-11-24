package admin

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/netguard"
	"github.com/fuseone/agents/internal/settings"
)

/*
A remote server's browser sign-in, as stored.

Its public half is the server row's value, beside the rest of the server, so
every reader of servers sees it without opening anything. The client secret,
when the client has one, is a row of its own (KindMCPOAuthLogin) at the same
scope and name, sealed there. Why apart: the server's sealed document is read
by every binary since bearer tokens, and one holding only a field an older
binary does not know reads, to it, as a bearer token made of JSON.

SecretMark names the secret without revealing it. The server's revision is
worked out from its own row alone; without the mark a secret changed by itself
would be a write that changes nothing, and be skipped, and the worker, which
notices a change by the row's updated_at, would never hear of it.
*/
type storedOAuthLogin struct {
	ClientID        string   `json:"clientID"`
	Scopes          []string `json:"scopes,omitempty"`
	AuthorizeURL    string   `json:"authorizeURL,omitempty"`
	TokenURL        string   `json:"tokenURL,omitempty"`
	Resource        string   `json:"resource,omitempty"`
	TokenAuthMethod string   `json:"tokenAuthMethod"`
	HasClientSecret bool     `json:"hasClientSecret,omitempty"`
	SecretMark      string   `json:"secretMark,omitempty"`
}

// loginMarkContext keys the secret's mark: the same wherever the server is
// stored, so moving it does not move its revision for this reason.
const loginMarkContext = "mcp_oauth_login"

func (s *storedOAuthLogin) login() *domain.MCPOAuthLogin {
	if s == nil {
		return nil
	}
	return &domain.MCPOAuthLogin{
		ClientID: s.ClientID, Scopes: slices.Clone(s.Scopes),
		AuthorizeURL: s.AuthorizeURL, TokenURL: s.TokenURL, Resource: s.Resource,
		TokenAuthMethod: s.TokenAuthMethod, HasClientSecret: s.HasClientSecret,
	}
}

// foldedLogin is a write's sign-in configuration, and what it does to the secret.
type foldedLogin struct {
	stored *storedOAuthLogin
	secret domain.LoginSecretChange
}

/*
foldLogin is the configuration a write leaves, judged on what will be stored:
the server it is for, the credential beside it and its scope's policy. A
configuration kept from before is judged too, so an edit cannot make a GitHub
App of a server people sign in to, nor keep endpoints a narrowed policy now
refuses.
*/
func (i *Integrations) foldLogin(
	was *storedOAuthLogin, w MCPServerWrite, merged domain.MCPCredentials, egress *domain.MCPEgress,
) (foldedLogin, error) {
	if w.Login == nil {
		if was == nil {
			return foldedLogin{}, nil
		}
		return foldedLogin{stored: was}, judgeLogin(was.login(), w.Server, merged, egress)
	}
	login, change, err := w.Login.Apply(was.login())
	if err != nil || login == nil {
		return foldedLogin{secret: change}, err
	}
	if err := judgeLogin(login, w.Server, merged, egress); err != nil {
		return foldedLogin{}, err
	}
	mark := ""
	switch {
	case change.Set != nil:
		mark = i.settings.Mark([]byte(*change.Set), loginMarkContext)
	case login.HasClientSecret && was != nil:
		mark = was.SecretMark
	}
	return foldedLogin{stored: storedOAuthLoginOf(login, mark), secret: change}, nil
}

func judgeLogin(
	login *domain.MCPOAuthLogin, server domain.MCPServer, merged domain.MCPCredentials, egress *domain.MCPEgress,
) error {
	switch {
	case server.TransportOf() != domain.TransportHTTP:
		return fmt.Errorf("%w: a browser sign-in is for a remote server", domain.ErrMCPOAuthLoginInvalid)
	case merged.GitHubApp != nil && !merged.GitHubApp.Empty():
		return fmt.Errorf("%w: a server acting as a GitHub App has no person to sign in", domain.ErrMCPOAuthLoginInvalid)
	}
	endpoints := nonEmpty(login.AuthorizeURL, login.TokenURL)
	for _, raw := range endpoints {
		if err := netguard.ValidateHTTPURL(raw); err != nil {
			return fmt.Errorf("%w: %v", domain.ErrMCPOAuthLoginInvalid, err)
		}
	}
	return originsWithin(egress, endpoints)
}

func storedOAuthLoginOf(login *domain.MCPOAuthLogin, mark string) *storedOAuthLogin {
	return &storedOAuthLogin{
		ClientID: login.ClientID, Scopes: slices.Clone(login.Scopes),
		AuthorizeURL: login.AuthorizeURL, TokenURL: login.TokenURL, Resource: login.Resource,
		TokenAuthMethod: login.TokenAuthMethod, HasClientSecret: login.HasClientSecret,
		SecretMark: mark,
	}
}

// writeLoginSecret carries out what a write did to the sealed secret, in the
// server write's own transaction, under the server name's lock every writer of
// either row takes first.
func (i *Integrations) writeLoginSecret(
	ctx context.Context, tx settings.DB, at settings.ScopeKind, stored domain.Scope,
	name string, by domain.UserID, change domain.LoginSecretChange,
) error {
	switch {
	case change.Set != nil:
		return i.settings.PutTx(ctx, tx, settings.Setting{
			ScopeKind: at, Scope: stored, Kind: settings.KindMCPOAuthLogin, Name: name,
			Secret: *change.Set, Enabled: true, UpdatedBy: string(by),
		})
	case change.Remove:
		return i.settings.DeleteTx(ctx, tx, at, stored, settings.KindMCPOAuthLogin, name)
	}
	return nil
}

/*
moveLoginSecret seals the secret again where the server moved: it is sealed
under its scope, and copied as it is would not open at the new one. Opened,
removed and written in the move's own transaction.
*/
func (i *Integrations) moveLoginSecret(
	ctx context.Context, tx settings.DB, name string, kind settings.ScopeKind, to domain.Scope, by domain.UserID,
) error {
	row, err := i.settings.RevealTx(ctx, tx, settings.ScopeInstallation, domain.Scope{}, settings.KindMCPOAuthLogin, name)
	if errors.Is(err, settings.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := i.settings.DeleteTx(ctx, tx, settings.ScopeInstallation, domain.Scope{}, settings.KindMCPOAuthLogin, name); err != nil {
		return err
	}
	return i.settings.PutTx(ctx, tx, settings.Setting{
		ScopeKind: kind, Scope: to, Kind: settings.KindMCPOAuthLogin, Name: name,
		Secret: row.Secret, Enabled: true, UpdatedBy: string(by),
	})
}

func nonEmpty(values ...string) []string {
	return slices.DeleteFunc(values, func(s string) bool { return s == "" })
}
