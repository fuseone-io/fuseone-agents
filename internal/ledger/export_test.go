package ledger

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// The fallback filters, exported to the plan test so it explains the exact
// text the aggregates run rather than a copy that could drift from it.
const (
	DeclaredPairRunsFilter  = declaredPairRuns
	DeclaredPairStepsFilter = declaredPairSteps
)

// EventDispatchBacklogSQL is the backlog statement, for the plan test that
// bounds what it reads.
const EventDispatchBacklogSQL = eventDispatchBacklogSQL

// TestMigration is one migration file, for a runner test that stages its own.
type TestMigration struct{ Version, Body string }

// MigrateBodies runs the production runner over the given files, as if they
// were the embedded set.
func MigrateBodies(ctx context.Context, pool *pgxpool.Pool, files []TestMigration) error {
	set := make([]embeddedMigration, 0, len(files))
	for _, f := range files {
		set = append(set, embeddedMigration{version: f.Version, body: []byte(f.Body), digest: digestBytes([]byte(f.Body))})
	}
	return migrateSet(ctx, pool, set, migrationLogger(nil))
}

// MigrationDigest is the digest the runner records for a body.
func MigrationDigest(body string) string { return digestBytes([]byte(body)) }

// FinishedSinceSQL is one page of the dispatcher's feed, for the same test.
const FinishedSinceSQL = finishedSinceSQL

// AppendInTx seals a step inside a transaction the test holds open, so a race
// can be staged on the production append rather than on a copy of its SQL.
func AppendInTx(ctx context.Context, tx pgx.Tx, s domain.Step) (domain.Step, error) {
	return appendTx(ctx, tx, nil, nil, s)
}

// MigrateThrough applies the embedded migrations up to and including version
// through the production runner, so an upgrade test can stand a database at
// the boundary a previous release left it at and let Migrate do the rest.
func MigrateThrough(ctx context.Context, pool *pgxpool.Pool, version string) error {
	all, err := loadMigrations()
	if err != nil {
		return err
	}
	n := 0
	for n < len(all) && all[n].version <= version {
		n++
	}
	if n == 0 || all[n-1].version != version {
		return fmt.Errorf("migration %s is not embedded", version)
	}
	return migrateSet(ctx, pool, all[:n], migrationLogger(nil))
}
