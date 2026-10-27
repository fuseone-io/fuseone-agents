package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// fakeMarks is the sweeper's memory, served as recorded.
type fakeMarks map[string]domain.BudgetMark

func (f fakeMarks) Announced(context.Context) (map[string]domain.BudgetMark, error) {
	return f, nil
}

// The mark is never erased — it is the sweeper's dedupe memory. What must not
// happen is the API serving it as a live alert after the operator raised the
// ceiling and the window rolled over (#146).
func TestListBudgetAlerts_aMarkTheBudgetOutgrew_isNotServed(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 27, 15, 0, 0, 0, time.UTC)
	crossed := domain.BudgetMark{
		Scope: adminScope, Threshold: 100,
		Since:       domain.PeriodDaily.Since(now.AddDate(0, 0, -5)),
		SpentMicros: 100_062_552, CeilingMicros: 100_000_000,
	}
	raised := domain.ScopeBudget{
		Scope: adminScope, Period: domain.PeriodDaily, Enabled: true,
		Budget: domain.Budget{Micros: 1_000_000_000},
	}

	resp, err := NewServer(ledger.NewMemory(), "test").
		WithMarks(fakeMarks{crossed.Key(): crossed}).
		WithCeilings(&fakeCeilings{configured: []domain.ScopeBudget{raised}}).
		WithClock(fixedAt{t: now}).
		ListBudgetAlerts(as(domain.RoleCurator), openapi.ListBudgetAlertsRequestObject{})
	if err != nil {
		t.Fatalf("ListBudgetAlerts: %v", err)
	}
	items := resp.(openapi.ListBudgetAlerts200JSONResponse).Items
	if len(items) != 0 {
		t.Fatalf("items = %+v, want none: the mark describes a ceiling that is gone", items)
	}
}

func TestListBudgetAlerts_aCrossingOfTheCurrentWindow_isServed(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 27, 15, 0, 0, 0, time.UTC)
	crossed := domain.BudgetMark{
		Scope: adminScope, Threshold: 80,
		Since:       domain.PeriodDaily.Since(now),
		SpentMicros: 85_000_000, CeilingMicros: 100_000_000,
	}
	ceiling := domain.ScopeBudget{
		Scope: adminScope, Period: domain.PeriodDaily, Enabled: true,
		Budget: domain.Budget{Micros: 100_000_000},
	}

	resp, err := NewServer(ledger.NewMemory(), "test").
		WithMarks(fakeMarks{crossed.Key(): crossed}).
		WithCeilings(&fakeCeilings{configured: []domain.ScopeBudget{ceiling}}).
		WithClock(fixedAt{t: now}).
		ListBudgetAlerts(as(domain.RoleCurator), openapi.ListBudgetAlertsRequestObject{})
	if err != nil {
		t.Fatalf("ListBudgetAlerts: %v", err)
	}
	items := resp.(openapi.ListBudgetAlerts200JSONResponse).Items
	if len(items) != 1 {
		t.Fatalf("items = %+v, want the one live crossing", items)
	}
	if items[0].Threshold != 80 || items[0].SpentMicros != 85_000_000 {
		t.Errorf("served %+v, want the recorded crossing", items[0])
	}
	// From the ceiling that validated the mark: a daily banner must not read
	// as a monthly one.
	if items[0].Period != openapi.BudgetAlertPeriodDaily {
		t.Errorf("period = %q, want daily from the configured ceiling", items[0].Period)
	}
}

func TestListBudgetAlerts_aMonthlyCeilingAnnouncesAMonthlyWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 27, 15, 0, 0, 0, time.UTC)
	crossed := domain.BudgetMark{
		Scope: adminScope, Threshold: 50,
		Since:       domain.PeriodMonthly.Since(now),
		SpentMicros: 55_000_000, CeilingMicros: 100_000_000,
	}
	ceiling := domain.ScopeBudget{
		Scope: adminScope, Period: domain.PeriodMonthly, Enabled: true,
		Budget: domain.Budget{Micros: 100_000_000},
	}

	resp, err := NewServer(ledger.NewMemory(), "test").
		WithMarks(fakeMarks{crossed.Key(): crossed}).
		WithCeilings(&fakeCeilings{configured: []domain.ScopeBudget{ceiling}}).
		WithClock(fixedAt{t: now}).
		ListBudgetAlerts(as(domain.RoleCurator), openapi.ListBudgetAlertsRequestObject{})
	if err != nil {
		t.Fatalf("ListBudgetAlerts: %v", err)
	}
	items := resp.(openapi.ListBudgetAlerts200JSONResponse).Items
	if len(items) != 1 {
		t.Fatalf("items = %+v, want the one live crossing", items)
	}
	if items[0].Period != openapi.BudgetAlertPeriodMonthly {
		t.Errorf("period = %q, want monthly from the configured ceiling", items[0].Period)
	}
}

func TestListBudgetAlerts_aMarkWhoseBudgetWasDeleted_isNotServed(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 27, 15, 0, 0, 0, time.UTC)
	crossed := domain.BudgetMark{
		Scope: adminScope, Threshold: 100,
		Since:       domain.PeriodDaily.Since(now),
		SpentMicros: 100_062_552, CeilingMicros: 100_000_000,
	}

	resp, err := NewServer(ledger.NewMemory(), "test").
		WithMarks(fakeMarks{crossed.Key(): crossed}).
		WithCeilings(&fakeCeilings{}).
		WithClock(fixedAt{t: now}).
		ListBudgetAlerts(as(domain.RoleCurator), openapi.ListBudgetAlertsRequestObject{})
	if err != nil {
		t.Fatalf("ListBudgetAlerts: %v", err)
	}
	items := resp.(openapi.ListBudgetAlerts200JSONResponse).Items
	if len(items) != 0 {
		t.Fatalf("items = %+v, want none: there is no budget to be short of", items)
	}
}
