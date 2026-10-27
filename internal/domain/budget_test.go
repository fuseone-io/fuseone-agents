package domain_test

import (
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

func TestHeadroom_exhaustedCeilingReportsOneNotZero(t *testing.T) {
	t.Parallel()

	// Zero means "no ceiling" everywhere else in this type. Reporting zero for
	// a spent budget would turn an exhausted area into an unlimited one.
	left := domain.Headroom(
		domain.Budget{Micros: 100},
		domain.Consumption{Micros: 250},
	)
	if left.Micros != 1 {
		t.Errorf("Micros = %d, want 1 so the next call is refused", left.Micros)
	}
}

func TestHeadroom_noCeilingStaysNoCeiling(t *testing.T) {
	t.Parallel()

	left := domain.Headroom(domain.Budget{}, domain.Consumption{Micros: 999})
	if left.Micros != 0 {
		t.Errorf("Micros = %d, want 0 meaning unlimited", left.Micros)
	}
}

func TestPeriod_monthlyWindowOpensOnTheFirst(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 11, 15, 30, 0, 0, time.UTC)
	if got := domain.PeriodMonthly.Since(now); !got.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Since = %v, want the first of the month", got)
	}
	if got := domain.PeriodDaily.Since(now); !got.Equal(time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Since = %v, want midnight", got)
	}
}

func TestDescribes_aMarkIsLiveOnlyWhileNothingItFrozeHasMoved(t *testing.T) {
	t.Parallel()

	// 2026-10-27, mid-afternoon, mid-month: a daily window and a monthly
	// window that are clearly distinct.
	now := time.Date(2026, 10, 27, 15, 0, 0, 0, time.UTC)
	scope := domain.Scope{Company: "acme", Area: "ops"}
	mark := func(since time.Time, ceiling int64) domain.BudgetMark {
		return domain.BudgetMark{
			Scope: scope, Threshold: 100, Since: since,
			SpentMicros: ceiling + 62552, CeilingMicros: ceiling,
		}
	}
	daily := func(micros int64) domain.ScopeBudget {
		return domain.ScopeBudget{
			Scope: scope, Period: domain.PeriodDaily, Enabled: true,
			Budget: domain.Budget{Micros: micros},
		}
	}
	today := domain.PeriodDaily.Since(now)

	cases := map[string]struct {
		mark    domain.BudgetMark
		ceiling domain.ScopeBudget
		want    bool
	}{
		"today's crossing of today's ceiling": {
			mark: mark(today, 100_000_000), ceiling: daily(100_000_000), want: true,
		},
		"the window rolled over": {
			mark:    mark(today.AddDate(0, 0, -5), 100_000_000),
			ceiling: daily(100_000_000), want: false,
		},
		"the ceiling was raised since": {
			mark: mark(today, 100_000_000), ceiling: daily(1_000_000_000), want: false,
		},
		"the budget was disabled": {
			mark: mark(today, 100_000_000),
			ceiling: domain.ScopeBudget{
				Scope: scope, Period: domain.PeriodDaily, Enabled: false,
				Budget: domain.Budget{Micros: 100_000_000},
			},
			want: false,
		},
		"the budget no longer caps money": {
			mark: mark(today, 100_000_000), ceiling: daily(0), want: false,
		},
		"the period changed from daily to monthly": {
			mark: mark(today, 100_000_000),
			ceiling: domain.ScopeBudget{
				Scope: scope, Period: domain.PeriodMonthly, Enabled: true,
				Budget: domain.Budget{Micros: 100_000_000},
			},
			want: false,
		},
		"this month's crossing of a monthly ceiling": {
			mark: mark(domain.PeriodMonthly.Since(now), 100_000_000),
			ceiling: domain.ScopeBudget{
				Scope: scope, Period: domain.PeriodMonthly, Enabled: true,
				Budget: domain.Budget{Micros: 100_000_000},
			},
			want: true,
		},
		"somebody else's ceiling": {
			mark: mark(today, 100_000_000),
			ceiling: domain.ScopeBudget{
				Scope: domain.Scope{Company: "acme", Area: "cx"}, Period: domain.PeriodDaily,
				Enabled: true, Budget: domain.Budget{Micros: 100_000_000},
			},
			want: false,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := c.mark.Describes(c.ceiling, now); got != c.want {
				t.Errorf("Describes = %v, want %v", got, c.want)
			}
		})
	}
}

func TestNarrow_theTighterCeilingWins(t *testing.T) {
	t.Parallel()

	// PRD §3.1: budgets inherit downwards and never widen. An area asking for
	// more than its company allows gets the company's.
	area := domain.Budget{Micros: 900, Steps: 10}
	company := domain.Budget{Micros: 500}

	got := area.Narrow(company)
	if got.Micros != 500 {
		t.Errorf("Micros = %d, want the company's tighter ceiling", got.Micros)
	}
	if got.Steps != 10 {
		t.Errorf("Steps = %d, want the area's, since the company set none", got.Steps)
	}
}
