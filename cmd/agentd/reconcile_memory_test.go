package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/memory"
)

/*
The deadline this command sets for itself ends the run without failing it.

The chart declares three deadlines and only the innermost is meant to fire: the
process stops, prints what it managed, and exits zero. It did not. Every error
came back the same way, so a sweep that worked for fourteen minutes failed the
release — and then Kubernetes retried it inside what was left of the pod's own
deadline, where the second attempt would be killed before it printed anything.

Nothing is lost by exiting zero. The walk is resumable, the runtime repairs a
row as it touches one, and the next release carries on from there.
*/
func TestReconcileOutcome_ownDeadline_endsTheRunWithoutFailingIt(t *testing.T) {
	t.Parallel()
	incomplete, err := outcome(fmt.Errorf("reconcile assertions: %w", context.DeadlineExceeded))
	if err != nil {
		t.Fatalf("outcome = %v, want the release to carry on", err)
	}
	if !incomplete {
		t.Error("the run was not reported as incomplete, so the log would claim it finished")
	}
}

// Everything else is still a failure. A store that will not answer and a cursor
// that does not move are things a person has to know about, and neither is what
// a deadline looks like.
func TestReconcileOutcome_anythingElse_failsTheRelease(t *testing.T) {
	t.Parallel()
	for _, failure := range []struct {
		name string
		err  error
	}{
		{"the store is away", errors.New("dial tcp: connection refused")},
		{"the cursor is stuck", fmt.Errorf("walk: %w", memory.ErrNoProgress)},
		{"somebody stopped it", fmt.Errorf("walk: %w", context.Canceled)},
	} {
		t.Run(failure.name, func(t *testing.T) {
			t.Parallel()
			incomplete, err := outcome(failure.err)
			if err == nil {
				t.Fatalf("outcome(%v) = nil, want the release to stop", failure.err)
			}
			if incomplete {
				t.Error("a failure was reported as a run that merely ran out of time")
			}
		})
	}
}

// A run that stopped early still says what it did. The counts are the only
// account of the work, and an operator who sees the deadline and no numbers
// cannot tell a sweep that repaired thousands from one that never started.
func TestReport_incompleteRun_keepsTheCountsAndSaysSo(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	report(slog.New(slog.NewTextHandler(&out, nil)), "memory",
		memory.Totals{Pages: 7, Scanned: 600, Repaired: 588, Conflicted: 2}, true)

	logged := out.String()
	for _, want := range []string{"incomplete=true", "repaired=588", "conflicted=2", "needs_review=true"} {
		if !strings.Contains(logged, want) {
			t.Errorf("log = %s, want it to carry %s", logged, want)
		}
	}
	if !strings.Contains(logged, "level=WARN") {
		t.Errorf("log = %s, want a warning rather than an ordinary line", logged)
	}
}

// And it carries counts only. A subject is what somebody typed into a memory
// and an assertion id is a handle to it; a release log is not where either
// belongs.
func TestReport_saysNothingAboutWhichRowsTheyWere(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	report(slog.New(slog.NewTextHandler(&out, nil)), "memory",
		memory.Totals{Pages: 2, Scanned: 100, Repaired: 4, Unproved: 1}, false)

	logged := out.String()
	for _, forbidden := range []string{"mem_", "mems_", "cursor", "subject", "claim"} {
		if strings.Contains(logged, forbidden) {
			t.Errorf("log = %s, want nothing naming a row: found %q", logged, forbidden)
		}
	}
}
