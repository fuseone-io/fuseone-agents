package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

// kindMCPOAuthFlow is a browser sign-in in flight: begun, not yet returned.
const kindMCPOAuthFlow settings.Kind = "mcp_oauth_flow"

/*
MCPOAuthFlow is a browser sign-in between the person asking to connect and
their browser coming back. State names it and is never stored as itself, only
its digest: the row's name is no use to whoever reads the table. Verifier is
the PKCE verifier, sealed. The rest is what the return is judged by and what
the code is exchanged with, fixed when the sign-in began.
*/
type MCPOAuthFlow struct {
	State     string
	Verifier  string
	Principal domain.UserID
	Server    string
	// ServerScope is the runtime scope of the server row the sign-in began
	// for, and ServerAt that row's revision then: a return to a server moved
	// or reconfigured since is refused.
	ServerScope     domain.Scope
	ServerAt        time.Time
	ClientID        string
	TokenAuthMethod string
	TokenURL        string
	Resource        string
	Issuer          string
	// IssParameter is the authorization server saying its answers name it
	// (RFC 9207): a return then without the issuer, or with another, is refused.
	IssParameter bool
	RedirectURI  string
	ExpiresAt    time.Time
}

type storedOAuthFlow struct {
	Principal       string    `json:"principal"`
	Server          string    `json:"server"`
	Scope           string    `json:"scope"`
	ServerAt        time.Time `json:"serverAt"`
	ClientID        string    `json:"clientID"`
	TokenAuthMethod string    `json:"tokenAuthMethod"`
	TokenURL        string    `json:"tokenURL"`
	Resource        string    `json:"resource,omitempty"`
	Issuer          string    `json:"issuer,omitempty"`
	IssParameter    bool      `json:"issParameter,omitempty"`
	RedirectURI     string    `json:"redirectURI"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

var (
	// ErrMCPOAuthFlowUnknown is a return for no sign-in in flight: never
	// begun, or already returned.
	ErrMCPOAuthFlowUnknown = errors.New("admin: no browser sign-in is in flight for this answer")
	// ErrMCPOAuthFlowNotYours is a return in another person's session.
	ErrMCPOAuthFlowNotYours = errors.New("admin: this sign-in was begun by someone else")
	// ErrMCPOAuthFlowExpired is a return after the sign-in's time ran out.
	ErrMCPOAuthFlowExpired = errors.New("admin: this sign-in took too long; start it again")
)

func flowName(state string) string {
	sum := sha256.Sum256([]byte(state))
	return hex.EncodeToString(sum[:])
}

// BeginMCPOAuthFlow records a sign-in in flight, and clears those nobody came
// back for. now is the caller's clock, the one the return is judged by.
func (i *Integrations) BeginMCPOAuthFlow(ctx context.Context, f MCPOAuthFlow, now time.Time) error {
	if f.State == "" || f.Verifier == "" || f.Principal == "" {
		return errors.New("admin: a sign-in needs its state, its verifier and its person")
	}
	scope, err := json.Marshal(f.ServerScope)
	if err != nil {
		return fmt.Errorf("admin: encode the sign-in's scope: %w", err)
	}
	value, err := json.Marshal(storedOAuthFlow{
		Principal: string(f.Principal), Server: f.Server, Scope: string(scope), ServerAt: f.ServerAt,
		ClientID: f.ClientID, TokenAuthMethod: f.TokenAuthMethod, TokenURL: f.TokenURL,
		Resource: f.Resource, Issuer: f.Issuer, IssParameter: f.IssParameter,
		RedirectURI: f.RedirectURI, ExpiresAt: f.ExpiresAt,
	})
	if err != nil {
		return fmt.Errorf("admin: encode the sign-in: %w", err)
	}
	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		delete from settings where kind = $1 and (value->>'expiresAt')::timestamptz < $2`,
		string(kindMCPOAuthFlow), now); err != nil {
		return fmt.Errorf("admin: clear the sign-ins nobody returned from: %w", err)
	}
	if err := i.settings.PutTx(ctx, tx, settings.Setting{
		ScopeKind: settings.ScopeInstallation, Kind: kindMCPOAuthFlow, Name: flowName(f.State),
		Value: value, Secret: f.Verifier, Enabled: true, UpdatedBy: string(f.Principal),
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("admin: commit: %w", err)
	}
	return nil
}

/*
ConsumeMCPOAuthFlow takes the sign-in a return names, whatever is then said
of it: held under its row's lock and removed in the same transaction, it is
used once, by one return, even when a browser comes back twice at once. Who
returned and when are judged after the lock, on the row as it is.
*/
func (i *Integrations) ConsumeMCPOAuthFlow(
	ctx context.Context, state string, principal domain.UserID, now time.Time,
) (MCPOAuthFlow, error) {
	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return MCPOAuthFlow{}, fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	name := flowName(state)
	row, err := i.settings.RevealTx(ctx, tx, settings.ScopeInstallation, domain.Scope{}, kindMCPOAuthFlow, name)
	if errors.Is(err, settings.ErrNotFound) {
		return MCPOAuthFlow{}, ErrMCPOAuthFlowUnknown
	}
	if err != nil {
		return MCPOAuthFlow{}, err
	}
	if err := i.settings.DeleteTx(ctx, tx, settings.ScopeInstallation, domain.Scope{}, kindMCPOAuthFlow, name); err != nil {
		return MCPOAuthFlow{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MCPOAuthFlow{}, fmt.Errorf("admin: commit: %w", err)
	}
	return judgeFlow(row, state, principal, now)
}

func judgeFlow(row settings.Setting, state string, principal domain.UserID, now time.Time) (MCPOAuthFlow, error) {
	var stored storedOAuthFlow
	if err := json.Unmarshal(row.Value, &stored); err != nil {
		return MCPOAuthFlow{}, fmt.Errorf("admin: decode the sign-in: %w", err)
	}
	var scope domain.Scope
	if err := json.Unmarshal([]byte(stored.Scope), &scope); err != nil {
		return MCPOAuthFlow{}, fmt.Errorf("admin: decode the sign-in's scope: %w", err)
	}
	switch {
	case stored.Principal != string(principal):
		return MCPOAuthFlow{}, ErrMCPOAuthFlowNotYours
	case !now.Before(stored.ExpiresAt):
		return MCPOAuthFlow{}, ErrMCPOAuthFlowExpired
	}
	return MCPOAuthFlow{
		State: state, Verifier: row.Secret, Principal: principal,
		Server: stored.Server, ServerScope: scope, ServerAt: stored.ServerAt,
		ClientID: stored.ClientID, TokenAuthMethod: stored.TokenAuthMethod, TokenURL: stored.TokenURL,
		Resource: stored.Resource, Issuer: stored.Issuer, IssParameter: stored.IssParameter,
		RedirectURI: stored.RedirectURI, ExpiresAt: stored.ExpiresAt,
	}, nil
}
