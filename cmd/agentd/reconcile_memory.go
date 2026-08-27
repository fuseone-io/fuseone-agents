package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/memory"
)

/*
reconcileMemory fills in what the platform can now derive about memory it wrote
before it could.

Run as a release hook, after the schema and before the new pods serve. The
runtime repairs a row as it touches it, which is the guarantee; this is what
stops that being paid one row at a time for ever, and what shortens the window
in which an old pod's write is the only unrepaired thing left.

Safe to run again, and safe to interrupt. A row whose citations already carry
their step, their source and their labels, and whose canonical identity is
stored, stops being a candidate — so a second run costs what is left rather than
what there was.
*/
func reconcileMemory(args []string) error {
	fs := flag.NewFlagSet("reconcile-memory", flag.ContinueOnError)
	dsn := fs.String("dsn", os.Getenv("DATABASE_URL"), "PostgreSQL connection string")
	timeout := fs.Duration("timeout", 15*time.Minute,
		"give up after this long; the deadline the release is willing to wait")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dsn == "" {
		return errors.New("reconcile-memory requires --dsn or DATABASE_URL")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	// One moment for the whole run. Every transition a sweep causes is dated by
	// it, and a clock read per page would file rows from one run under times
	// that drift apart while it is working.
	started := time.Now().UTC()
	resolver := memory.NewResolver(ledger.NewPostgres(pool), ledger.NewContent(pool))

	assertions, suggestions, err := memory.Reconcile(
		ctx, memory.NewPostgres(pool), resolver, started)
	incomplete, fatal := outcome(err)
	// Reported either way. A run that stopped halfway still repaired what it
	// reached, and an operator reading only the error would think it had not.
	report("memory", assertions, incomplete)
	report("memory suggestions", suggestions, incomplete)
	return fatal
}

/*
outcome is what a run means to the release.

The deadline this command set for itself is a stopping point, not a failure. The
work is resumable and the runtime repairs a row as it touches one, so a sweep
that ran out of time has done real work and left the rest for the next release —
and failing here would be worse than useless: Kubernetes would retry inside what
is left of the pod's own deadline, and the second attempt would be killed before
it reached the point where it prints anything.

Everything else still fails. A store that will not answer and a cursor that does
not move are both things a person has to know about, and neither is what a
deadline looks like. So is cancellation: nothing cancels this today, but a run
somebody stopped is not a run that finished early.
*/
func outcome(err error) (incomplete bool, fatal error) {
	switch {
	case err == nil:
		return false, nil
	case errors.Is(err, context.DeadlineExceeded):
		return true, nil
	}
	return false, fmt.Errorf("reconcile memory: %w", err)
}

/*
report says what a sweep did, in counts and nothing else.

No identifiers, no subjects, no cursors. A subject is what somebody typed into a
memory and an assertion id is a handle to it, and an operator deciding whether
to look needs neither — this log is read by whoever is watching a release, not
by whoever will resolve what it found.

Rows needing a person are a warning rather than a failure. Neither improves with
a retry: a conflicted identity is two rows somebody has to choose between, and
an unproved citation is evidence the ledger will not vouch for. Exiting non-zero
would fail the release, repeat the same reading until the backoff limit, and
resolve nothing.

A run cut short by its own deadline is a warning too, and for the same reason:
the counts are what it managed, and the next release carries on from there.
*/
func report(what string, totals memory.Totals, incomplete bool) {
	fields := []any{
		"table", what,
		"pages", totals.Pages,
		"scanned", totals.Scanned,
		"repaired", totals.Repaired,
		"source_gone", totals.SourceGone,
	}
	if totals.NeedsReview() {
		fields = append(fields,
			"conflicted", totals.Conflicted,
			"unproved", totals.Unproved,
			"needs_review", true)
	}
	switch {
	case incomplete:
		slog.Warn("memory reconciliation stopped at its deadline; the next run continues",
			append(fields, "incomplete", true)...)
	case totals.NeedsReview():
		slog.Warn("memory reconciled, and some rows need a person", fields...)
	default:
		slog.Info("memory reconciled", fields...)
	}
}
