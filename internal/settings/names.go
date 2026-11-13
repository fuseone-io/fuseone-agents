package settings

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fuseone/agents/internal/domain"
)

// integrationNames is the index that keeps an integration's name unique
// across scopes (migration 0108).
const integrationNames = "settings_integration_names"

// nameTaken reports a write refused by that index, and nothing else: a unique
// violation of the primary key is an upsert's own row and cannot happen here.
func nameTaken(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == integrationNames
}

/*
RevealNamed is Reveal of the one row holding a name, at whatever scope it is
stored, for a kind whose names are unique across scopes (migration 0108).

One statement, so the scope it finds and the row it opens are one snapshot: a
lookup then a read let a write land between them, and the answer was not the
state the read began on. The credential is opened under the scope the row
itself names.
*/
func (s *Store) RevealNamed(ctx context.Context, kind Kind, name string) (Setting, error) {
	return s.RevealNamedIf(ctx, kind, name, nil)
}

/*
RevealNamedIf is RevealNamed that asks first, with the row's scope in hand,
whether its credential may be opened at all.

Still one statement, so the answer is the state the read began on. allow runs
on the scope the row names before anything is decrypted: a caller refused the
row is refused there, and never hears what opening it would have said — such as
a credential the vault cannot open, which an error would otherwise tell apart
from a row that does not exist. A nil allow opens every row.
*/
func (s *Store) RevealNamedIf(
	ctx context.Context, kind Kind, name string, allow func(ScopeKind, domain.Scope) error,
) (Setting, error) {
	out := Setting{Kind: kind, Name: name}
	var scopeKind, company, area string
	var ciphertext, nonce []byte
	err := s.pool.QueryRow(ctx, `
		select scope_kind, company_id, area_id, value, secret, secret_nonce, enabled, updated_by, updated_at
		from settings where kind = $1 and name = $2`, string(kind), name,
	).Scan(&scopeKind, &company, &area, &out.Value, &ciphertext, &nonce, &out.Enabled, &out.UpdatedBy, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Setting{}, fmt.Errorf("%w: %s/%s", ErrNotFound, kind, name)
	}
	if err != nil {
		return Setting{}, fmt.Errorf("settings: read %s/%s: %w", kind, name, err)
	}
	out.ScopeKind = ScopeKind(scopeKind)
	out.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	if allow != nil {
		if err := allow(out.ScopeKind, out.Scope); err != nil {
			return Setting{}, err
		}
	}
	return s.open(out, ciphertext, nonce)
}
