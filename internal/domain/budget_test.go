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
