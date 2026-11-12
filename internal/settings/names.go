package settings

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
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
