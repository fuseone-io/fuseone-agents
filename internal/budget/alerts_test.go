package budget_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/budget"
	"github.com/fuseone/agents/internal/domain"
)

// A hard limit that says nothing until it stops the work is a limit discovered
// by a run parking mid-afternoon. These are about saying it earlier, and about
// saying it once.

type ceilings []domain.ScopeBudget

func (c ceilings) List(context.Context) ([]domain.ScopeBudget, error) { return c, nil }

type spend int64

func (s spend) SpentSince(context.Context, domain.Scope, time.Time) (domain.Consumption, error) {
	return domain.Consumption{Micros: int64(s)}, nil
}

type marks map[string]domain.BudgetMark

func (m marks) Announced(context.Context) (map[string]domain.BudgetMark, error) {
	return m, nil
}

func (m marks) Announce(_ context.Context, mark domain.BudgetMark) error {
	m[mark.Key()] = mark
	return nil
}

type clock struct{ t time.Time }

func (c clock) Now() time.Time { return c.t }

func monthly(micros int64) ceilings {
	return ceilings{{
		ScopeKind: "area", Scope: domain.Scope{Company: "acme", Area: "cx"},
		Period: domain.PeriodMonthly, Budget: domain.Budget{Micros: micros},
		Enabled: true,
	}}
}

func sweep(t *testing.T, spent int64, seen marks) []domain.BudgetMark {
	t.Helper()
	w := budget.NewWatcher(monthly(1_000_000), spend(spent), seen,
		clock{t: time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)}, nil)
	crossed, err := w.Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	return crossed
}

func TestSweep_belowTheFirstThreshold_saysNothing(t *testing.T) {
	t.Parallel()

	if got := sweep(t, 400_000, marks{}); len(got) != 0 {
		t.Errorf("announced %+v at 40%%, want nothing", got)
	}
}

func TestSweep_atHalf_announcesOnce(t *testing.T) {
	t.Parallel()
	seen := marks{}

	first := sweep(t, 500_000, seen)
	if len(first) != 1 || first[0].Threshold != 50 {
		t.Fatalf("announced %+v, want one crossing at 50", first)
	}

	// The sweep runs every few minutes. A warning that repeated every pass
	// would be read once and filtered for ever after.
	if again := sweep(t, 520_000, seen); len(again) != 0 {
		t.Errorf("announced %+v on the second pass, want silence", again)
	}
}

func TestSweep_jumpingStraightToTheCeiling_saysItOnce(t *testing.T) {
	t.Parallel()

	// A scope that goes from nothing to spent in one busy hour is at its
	// ceiling. Telling it three things at once buries the one that matters.
	got := sweep(t, 1_000_000, marks{})

	if len(got) != 1 || got[0].Threshold != 100 {
		t.Errorf("announced %+v, want one crossing at 100", got)
	}
}

func TestSweep_climbingHigher_announcesTheNewThreshold(t *testing.T) {
	t.Parallel()
	seen := marks{}

	sweep(t, 500_000, seen)
	got := sweep(t, 850_000, seen)

	if len(got) != 1 || got[0].Threshold != 80 {
		t.Errorf("announced %+v at 85%%, want the 80 crossing", got)
	}
}

func TestSweep_aNewPeriod_warnsAgainFromTheStart(t *testing.T) {
	t.Parallel()

	// The only reading of a monthly budget that works. A mark carried across
	// the boundary would silence every month after the first.
	seen := marks{}
	sweep(t, 900_000, seen)

	next := budget.NewWatcher(monthly(1_000_000), spend(600_000), seen,
		clock{t: time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)}, nil)
	got, err := next.Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if len(got) != 1 || got[0].Threshold != 50 {
		t.Errorf("announced %+v in the new month, want the 50 crossing", got)
	}
}

func TestSweep_aCeilingWithNoAmount_isNotWatched(t *testing.T) {
	t.Parallel()

	// The other dimensions are per run rather than per period: a scope has no
	// meaningful "80% of its steps".
	w := budget.NewWatcher(monthly(0), spend(9_999_999), marks{},
		clock{t: time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)}, nil)

	got, err := w.Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("announced %+v for a ceiling with no amount", got)
	}
}
