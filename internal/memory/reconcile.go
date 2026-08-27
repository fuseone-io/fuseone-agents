package memory

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrNoProgress means a sweep asked for the page after a cursor and was handed
// the same cursor back. Nothing advanced, so continuing is a loop, and the job
// stops rather than spending its deadline discovering that.
var ErrNoProgress = errors.New("memory: the reconciliation cursor did not advance")

// reconcilePage is how many rows one page holds. Not a setting: it is the same
// bound a list is held to, and an operator choosing it would be choosing how
// long one transaction stays open without anything telling them what that
// costs.
const reconcilePage = 100

/*
Sweeps is what a reconciliation walks — the two tables, each on a cursor of its
own.

Declared here rather than taken as a concrete store so the loop can be tested
against the in-memory one. What it needs is small on purpose: a reconciliation
reads and repairs, and a type that cannot accept or dismiss cannot do either by
accident.
*/
type Sweeps interface {
	Hydrate(context.Context, *Resolver, HydratePage) (HydrateResult, error)
	HydrateSuggestions(context.Context, *Resolver, HydratePage) (HydrateResult, error)
}

/*
Totals are what one reconciliation did, added up across its pages.

Aggregate on purpose. Which rows they were is the kind of thing a log should not
carry — a subject is what somebody typed into a memory and an assertion id is a
handle to it — and an operator deciding whether to look does not need either.
*/
type Totals struct {
	Pages      int
	Scanned    int
	Repaired   int
	SourceGone int
	Conflicted int
	Unproved   int
}

/*
NeedsReview is true when the sweep met rows it cannot finish on its own.

Neither is a failure of the job. A conflicted identity is two rows a person has
to choose between, and an unproved citation is evidence the ledger will not
vouch for; running again changes neither, so exiting non-zero would fail a
release, retry the same reading until the backoff limit, and resolve nothing.
What it does need is to be visible, which is what this is for.
*/
func (t Totals) NeedsReview() bool { return t.Conflicted > 0 || t.Unproved > 0 }

func (t *Totals) add(out HydrateResult) {
	t.Pages++
	t.Scanned += out.Scanned
	t.Repaired += out.Repaired
	t.SourceGone += out.SourceGone
	t.Conflicted += out.Conflicted
	t.Unproved += out.Unproved
}

/*
Reconcile walks both tables to the end and reports what it did.

One moment for every page, taken once by the caller: the transitions a sweep
causes are dated by it, and a clock read per page would file rows from the same
run under times that drift apart while the job is running.

Idempotent, which is what makes a restart cheap: a row whose citations already
carry their step, their source and their labels, and whose canonical identity is
stored, is not a candidate any more. A sweep interrupted halfway can begin again
from the start and only pay for what is left.
*/
func Reconcile(
	ctx context.Context, s Sweeps, r *Resolver, now time.Time,
) (assertions, suggestions Totals, err error) {
	assertions, err = walk(ctx, r, now, s.Hydrate)
	if err != nil {
		return assertions, suggestions, fmt.Errorf("reconcile assertions: %w", err)
	}
	// A cursor of its own, and a second pass rather than one interleaved with
	// the first: the two tables have different populations and one finishing
	// says nothing about the other.
	suggestions, err = walk(ctx, r, now, s.HydrateSuggestions)
	if err != nil {
		return assertions, suggestions, fmt.Errorf("reconcile suggestions: %w", err)
	}
	return assertions, suggestions, nil
}

func walk(
	ctx context.Context, r *Resolver, now time.Time,
	page func(context.Context, *Resolver, HydratePage) (HydrateResult, error),
) (Totals, error) {
	var totals Totals
	cursor := ""
	for {
		if err := ctx.Err(); err != nil {
			return totals, err
		}
		out, err := page(ctx, r, HydratePage{After: cursor, Limit: reconcilePage, Now: now})
		if err != nil {
			return totals, err
		}
		totals.add(out)
		if out.Cursor == "" {
			return totals, nil
		}
		// Ids sort, and a page always ends on one after where it started. Being
		// handed the same cursor back means the sweep is not moving, and a job
		// that cannot say so would spend its whole deadline proving it.
		if out.Cursor <= cursor {
			return totals, ErrNoProgress
		}
		cursor = out.Cursor
	}
}

// Reconciler is the durable store, named by what the job needs of it.
var _ Sweeps = (*Postgres)(nil)

// The in-memory store answers the same shape, which is what lets the loop above
// be tested without a database.
var _ Sweeps = (*Memory)(nil)
