package settings

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
Opening what is sealed.

Separate from reading what is stored, because they are different acts with
different consequences: a listing tells a screen that a credential exists, and
these hand one to something about to speak to a third party.
*/

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
	return s.ListTx(ctx, s.pool, kind)
}

// ListTx is List inside somebody else's transaction.
//
// Not a convenience: a caller that has to decide something from what is stored
// and then write must read under the same lock it writes under, or it decides
// from a state that no longer holds by the time it acts.
func (s *Store) ListTx(ctx context.Context, conn DB, kind Kind) ([]Setting, error) {
	rows, err := conn.Query(ctx, `
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

/*
RevealAll lists a kind with every credential opened, in one query.

One query because a configuration and the credential that goes with it are a
pair: read apart they are two rows read a moment apart, and an edit landing
between them hands the new credential to the old address — during a rotation
away from an endpoint somebody no longer trusts, that is the replacement key
delivered to exactly the place it was meant to leave.

It is also the difference between one round trip and one per row, on a path
that runs in every process every thirty seconds.

Nothing here is logged, and the secret is in the answer: this is for the wiring
that has to open credentials to build a client. A caller that only needs to know
whether one exists wants List.

A credential this process cannot open is described on its own row and never
returned as the collection's error. One sealed row used to cost the caller every
other row — names included — which is how a process ends up with no
configuration to honour and fills the gap from its environment.

A switched-off row's credential is not opened at all: nothing is going to speak
with it.
*/
func (s *Store) RevealAll(ctx context.Context, kind Kind) ([]Setting, error) {
	rows, err := s.pool.Query(ctx, `
		select scope_kind, company_id, area_id, name, value, secret, secret_nonce,
		       enabled, updated_by, updated_at
		from settings where kind = $1
		order by scope_kind, company_id, area_id, name`, string(kind))
	if err != nil {
		return nil, fmt.Errorf("settings: reveal %s: %w", kind, err)
	}
	defer rows.Close()

	var out []Setting
	for rows.Next() {
		var (
			set               = Setting{Kind: kind}
			scopeKind         string
			company, area     string
			ciphertext, nonce []byte
		)
		if err := rows.Scan(&scopeKind, &company, &area, &set.Name, &set.Value,
			&ciphertext, &nonce, &set.Enabled, &set.UpdatedBy, &set.UpdatedAt); err != nil {
			return nil, err
		}
		set.ScopeKind = ScopeKind(scopeKind)
		set.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
		set.HasSecret = ciphertext != nil
		switch {
		case !set.HasSecret:
		case !set.Enabled:
			// Nothing is going to use it. Opening a credential that will not
			// be spoken with is a decryption this process has no reason to
			// perform, and a failure it has no reason to report.
		case s.vault == nil:
			set.SecretUnreadable = ErrNoVault.Error()
		default:
			plain, err := s.vault.Open(ciphertext, nonce, contextFor(set))
			if err != nil {
				// Described, not returned. Returned, one sealed row cost the
				// caller every other row — including the names, which is how a
				// process ends up with no configuration to honour and fills
				// the gap from its environment.
				set.SecretUnreadable = err.Error()
				break
			}
			set.Secret = string(plain)
		}
		out = append(out, set)
	}
	return out, rows.Err()
}
