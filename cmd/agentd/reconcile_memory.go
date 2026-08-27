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
	// Reported either way. A run that failed halfway still repaired what it
	// reached, and an operator reading only the error would think it had not.
	report("memory", assertions)
	report("memory suggestions", suggestions)
	if err != nil {
		return fmt.Errorf("reconcile memory: %w", err)
	}
	return nil
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
*/
func report(what string, totals memory.Totals) {
	fields := []any{
		"table", what,
		"pages", totals.Pages,
		"scanned", totals.Scanned,
		"repaired", totals.Repaired,
		"source_gone", totals.SourceGone,
	}
	if !totals.NeedsReview() {
		slog.Info("memory reconciled", fields...)
		return
	}
	slog.Warn("memory reconciled, and some rows need a person",
		append(fields,
			"conflicted", totals.Conflicted,
			"unproved", totals.Unproved,
			"needs_review", true)...)
}
