// Package settings holds everything the administration area configures.
//
// Model providers, MCP servers, capability packs, ceilings and retention all
// live here rather than in the environment. A value in the environment cannot
// be audited, cannot be scoped to a company, and cannot change without a
// deploy — which is how a platform's most consequential decisions end up in a
// values file nobody reviews.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/vault"
)

var (
	ErrNotFound = errors.New("settings: no such setting")
	ErrNoVault  = errors.New("settings: a secret was supplied but no vault is configured")
)

// Kind groups settings by what they configure. Each kind has its own value
// shape, decoded by whichever package owns it.
type Kind string

const (
	KindModelProvider     Kind = "model_provider"
	KindMCPServer         Kind = "mcp_server"
	KindMCPUserCredential Kind = "mcp_user_credential"
	KindConnectorInstance Kind = "connector_instance"
	KindPack              Kind = "capability_pack"
	KindBudget            Kind = "budget"
	KindMoney             Kind = "money"
	KindRetention         Kind = "retention"
	KindBranding          Kind = "branding"
	KindPolicy            Kind = "policy"
	// KindIdentityProvider holds who may sign in and what signing in grants.
	// Here rather than in a table of its own because the vault, the trail and
	// the reveal-once path all already work for a setting, and an identity
	// provider needs all three.
	KindIdentityProvider Kind = "identity_provider"
	// KindSigningKey holds the key exports are signed with. Its public half
	// is not a secret — publishing it is what makes an export checkable by
	// somebody who does not trust this installation.
	KindSigningKey Kind = "signing_key"
)

// ScopeKind is the level a setting applies at.
//
// Resolution walks from the most specific level outward, so an area can
// tighten what its company allows without the company having to enumerate
// every area.
type ScopeKind string

const (
	ScopeInstallation ScopeKind = "installation"
	ScopeCompany      ScopeKind = "company"
	ScopeArea         ScopeKind = "area"
)

// Setting is one configured value.
type Setting struct {
	ScopeKind ScopeKind
	Scope     domain.Scope
	Kind      Kind
	Name      string

	// Value is the non-secret configuration: endpoints, prices, tool lists.
	Value json.RawMessage
	// Secret is the plaintext credential. It is present only when writing or
	// when a caller explicitly asked to reveal it, and is never populated by
	// an ordinary read.
	Secret string
	// HasSecret reports whether a credential is stored, without exposing it.
	HasSecret bool
	/*
		ClearSecret removes the stored credential.

		Separate from writing an empty one, because an omitted secret has to go
		on meaning "keep what is there" — re-entering a key to change an
		unrelated field is how operators end up pasting credentials into chat
		to look them up. With only that rule, though, a credential could be
		written and never taken back: the coalesce below preserves whatever is
		stored, so "clear it" and "do not mention it" were the same request.

		They are not. One of them is somebody removing a token from a server
		that no longer uses it.
	*/
	ClearSecret bool

	Enabled   bool
	UpdatedBy string
	UpdatedAt time.Time
}

// DB is what both a pool and a transaction satisfy.
//
// It exists so a configuration change can join the transaction that records
// who made it: a platform able to lose the record of a change while keeping
// the change is one nobody can audit.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store reads and writes configuration, sealing credentials on the way in.
type Store struct {
	pool  *pgxpool.Pool
	vault *vault.Vault
}

func NewStore(pool *pgxpool.Pool, v *vault.Vault) *Store {
	return &Store{pool: pool, vault: v}
}

// Put writes a setting, encrypting any credential before it reaches the
// database.
func (s *Store) Put(ctx context.Context, set Setting) error {
	return s.PutTx(ctx, s.pool, set)
}

// PutTx is Put inside a caller's transaction.
func (s *Store) PutTx(ctx context.Context, conn DB, set Setting) error {
	var ciphertext, nonce []byte
	if set.Secret != "" {
		if s.vault == nil {
			return ErrNoVault
		}
		var err error
		ciphertext, nonce, err = s.vault.Seal([]byte(set.Secret), contextFor(set))
		if err != nil {
			return err
		}
	}

	value := set.Value
	if len(value) == 0 {
		value = json.RawMessage(`{}`)
	}

	// A write that omits the secret keeps the stored one. Re-entering a
	// credential to change an unrelated field is how operators end up pasting
	// keys into chat to look them up.
	_, err := conn.Exec(ctx, `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, secret, secret_nonce, enabled, updated_by, updated_at)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now())
		on conflict (scope_kind, company_id, area_id, kind, name) do update set
			value        = excluded.value,
			secret       = case when $11 then null else coalesce(excluded.secret, settings.secret) end,
			secret_nonce = case when $11 then null else coalesce(excluded.secret_nonce, settings.secret_nonce) end,
			enabled      = excluded.enabled,
			updated_by   = excluded.updated_by,
			updated_at   = now()`,
		string(set.ScopeKind), string(set.Scope.Company), string(set.Scope.Area),
		string(set.Kind), set.Name, value, ciphertext, nonce, set.Enabled, set.UpdatedBy,
		set.ClearSecret)
	if err != nil {
		return fmt.Errorf("settings: write %s/%s: %w", set.Kind, set.Name, err)
	}
	return nil
}

// Get reads one setting without its credential.
func (s *Store) Get(ctx context.Context, scopeKind ScopeKind, scope domain.Scope, kind Kind, name string) (Setting, error) {
	row := s.pool.QueryRow(ctx, `
		select value, secret is not null, enabled, updated_by, updated_at
		from settings
		where scope_kind = $1 and company_id = $2 and area_id = $3 and kind = $4 and name = $5`,
		string(scopeKind), string(scope.Company), string(scope.Area), string(kind), name)

	out := Setting{ScopeKind: scopeKind, Scope: scope, Kind: kind, Name: name}
	err := row.Scan(&out.Value, &out.HasSecret, &out.Enabled, &out.UpdatedBy, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Setting{}, fmt.Errorf("%w: %s/%s", ErrNotFound, kind, name)
	}
	if err != nil {
		return Setting{}, fmt.Errorf("settings: read %s/%s: %w", kind, name, err)
	}
	return out, nil
}

// Reveal returns a setting with its credential decrypted.
//
// Separate from Get on purpose. Reading configuration is routine; reading a
// credential is not, and a caller has to ask for it explicitly so the audit
// trail can record that they did.
func (s *Store) Reveal(ctx context.Context, scopeKind ScopeKind, scope domain.Scope, kind Kind, name string) (Setting, error) {
	set, err := s.Get(ctx, scopeKind, scope, kind, name)
	if err != nil {
		return Setting{}, err
	}
	if !set.HasSecret {
		return set, nil
	}
	if s.vault == nil {
		return Setting{}, ErrNoVault
	}

	var ciphertext, nonce []byte
	if err := s.pool.QueryRow(ctx, `
		select secret, secret_nonce from settings
		where scope_kind = $1 and company_id = $2 and area_id = $3 and kind = $4 and name = $5`,
		string(scopeKind), string(scope.Company), string(scope.Area), string(kind), name,
	).Scan(&ciphertext, &nonce); err != nil {
		return Setting{}, fmt.Errorf("settings: read secret: %w", err)
	}

	plain, err := s.vault.Open(ciphertext, nonce, contextFor(set))
	if err != nil {
		return Setting{}, err
	}
	set.Secret = string(plain)
	return set, nil
}

/*
RevealTx is Reveal inside a caller's transaction, holding the row.

For a write that folds onto what is stored — keeping a credential a request did
not mention, or a choice it said nothing about. Read outside the transaction,
that fold is a lost update waiting for two people: one narrows a server, the
other saves a token having read the older value, and the second commit puts the
older value back. The row lock is what makes "keep what is there" mean what is
there when the write happens.

A row that does not exist locks nothing, and two concurrent creations of the
same name then serialise on the unique index instead — one wins wholesale,
which is the honest outcome when neither had anything to fold onto.
*/
func (s *Store) RevealTx(
	ctx context.Context, conn DB,
	scopeKind ScopeKind, scope domain.Scope, kind Kind, name string,
) (Setting, error) {
	out := Setting{ScopeKind: scopeKind, Scope: scope, Kind: kind, Name: name}
	var ciphertext, nonce []byte
	err := conn.QueryRow(ctx, `
		select value, secret, secret_nonce, enabled, updated_by, updated_at
		from settings
		where scope_kind = $1 and company_id = $2 and area_id = $3 and kind = $4 and name = $5
		for update`,
		string(scopeKind), string(scope.Company), string(scope.Area), string(kind), name,
	).Scan(&out.Value, &ciphertext, &nonce, &out.Enabled, &out.UpdatedBy, &out.UpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return Setting{}, fmt.Errorf("%w: %s/%s", ErrNotFound, kind, name)
	}
	if err != nil {
		return Setting{}, fmt.Errorf("settings: read %s/%s: %w", kind, name, err)
	}
	out.HasSecret = len(ciphertext) > 0
	if !out.HasSecret {
		return out, nil
	}
	if s.vault == nil {
		return Setting{}, ErrNoVault
	}
	plain, err := s.vault.Open(ciphertext, nonce, contextFor(out))
	if err != nil {
		return Setting{}, err
	}
	out.Secret = string(plain)
	return out, nil
}

// List returns every setting of a kind, without credentials.
func (s *Store) List(ctx context.Context, kind Kind) ([]Setting, error) {
	rows, err := s.pool.Query(ctx, `
		select scope_kind, company_id, area_id, name, value, secret is not null, enabled, updated_by, updated_at
		from settings where kind = $1
		order by scope_kind, company_id, area_id, name`, string(kind))
	if err != nil {
		return nil, fmt.Errorf("settings: list %s: %w", kind, err)
	}
	defer rows.Close()

	var out []Setting
	for rows.Next() {
		var (
			set           = Setting{Kind: kind}
			scopeKind     string
			company, area string
		)
		if err := rows.Scan(&scopeKind, &company, &area, &set.Name, &set.Value,
			&set.HasSecret, &set.Enabled, &set.UpdatedBy, &set.UpdatedAt); err != nil {
			return nil, err
		}
		set.ScopeKind = ScopeKind(scopeKind)
		set.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
		out = append(out, set)
	}
	return out, rows.Err()
}

// Resolve finds the setting that applies in a scope, walking outward.
//
// Area first, then company, then installation. This is what lets an area
// tighten a ceiling its company set without the company enumerating every
// area — and it is why a setting is never merged: the most specific one wins
// whole, so an operator reading one row knows what applies.
func (s *Store) Resolve(ctx context.Context, scope domain.Scope, kind Kind, name string) (Setting, error) {
	attempts := []struct {
		kind  ScopeKind
		scope domain.Scope
	}{
		{ScopeArea, scope},
		{ScopeCompany, domain.Scope{Company: scope.Company}},
		{ScopeInstallation, domain.Scope{}},
	}

	for _, a := range attempts {
		set, err := s.Get(ctx, a.kind, a.scope, kind, name)
		if err == nil {
			return set, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return Setting{}, err
		}
	}
	return Setting{}, fmt.Errorf("%w: %s/%s in %s", ErrNotFound, kind, name, scope)
}

// Delete removes a setting and, with it, any credential it held.
func (s *Store) Delete(ctx context.Context, scopeKind ScopeKind, scope domain.Scope, kind Kind, name string) error {
	return s.DeleteTx(ctx, s.pool, scopeKind, scope, kind, name)
}

// DeleteTx is Delete inside a caller's transaction.
func (s *Store) DeleteTx(ctx context.Context, conn DB, scopeKind ScopeKind, scope domain.Scope, kind Kind, name string) error {
	_, err := conn.Exec(ctx, `
		delete from settings
		where scope_kind = $1 and company_id = $2 and area_id = $3 and kind = $4 and name = $5`,
		string(scopeKind), string(scope.Company), string(scope.Area), string(kind), name)
	if err != nil {
		return fmt.Errorf("settings: delete %s/%s: %w", kind, name, err)
	}
	return nil
}

// contextFor binds a credential to the record that owns it.
//
// A row copied from one provider onto another fails to decrypt, so an attacker
// with write access to the database cannot promote a low-privilege credential
// by moving it somewhere more powerful.
func contextFor(s Setting) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s",
		s.ScopeKind, s.Scope.Company, s.Scope.Area, s.Kind, s.Name)
}
