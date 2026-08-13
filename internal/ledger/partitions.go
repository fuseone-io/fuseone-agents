package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

/*
Keeping months ahead of the clock.

The ledger is partitioned by the month a run opened, and a month with no
partition is not an error — everything outside the declared range lands in the
default partition and is recorded correctly. What it costs is archival: a
default partition cannot be detached as a month, and rows cannot be moved out
of it afterwards because the table is append-only and the trigger that makes it
so does not make exceptions for housekeeping.

So the months are made in advance, and generously. Twelve of them, every pass:
an installation that is down for a quarter comes back to somewhere to write,
and one that has been running for years pays a no-op for the eleven that
already exist.
*/

// MonthsAhead is how far ahead partitions are created.
//
// Wide on purpose. The cost of an unused empty partition is a catalogue row;
// the cost of a month arriving without one is a year of steps that can never
// be archived as a unit.
const MonthsAhead = 12

// Partitions makes the months a ledger is about to need.
type Partitions struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewPartitions(pool *pgxpool.Pool, now func() time.Time) *Partitions {
	return &Partitions{pool: pool, now: now}
}

// Ensure creates any missing month from this one forward, and answers how many
// it had to make. Safe to call repeatedly: creating a month that exists is a
// no-op in the database, not an error.
func (p *Partitions) Ensure(ctx context.Context) (int, error) {
	made := 0
	month := p.now().UTC()
	for range MonthsAhead {
		var name string
		if err := p.pool.QueryRow(ctx,
			`select run_steps_month($1::date)`, month).Scan(&name); err != nil {
			return made, fmt.Errorf("ledger: month %s: %w", month.Format("2006-01"), err)
		}
		made++
		month = month.AddDate(0, 1, 0)
	}
	return made, nil
}

// Stranded counts steps in the default partition.
//
// Not an error and not a corruption — those rows are read, verified and
// constrained like any other. It is a fact an operator should be able to see,
// because it means a month of this ledger cannot be detached and moved, and
// the only way it happens is the partition job having been stopped for a year.
func (p *Partitions) Stranded(ctx context.Context) (int64, error) {
	var n int64
	err := p.pool.QueryRow(ctx, `select count(*) from run_steps_default`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("ledger: count stranded steps: %w", err)
	}
	return n, nil
}
