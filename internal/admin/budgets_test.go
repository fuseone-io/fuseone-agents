package admin_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

func newBudgets(t *testing.T) *admin.Budgets {
	t.Helper()

	pool := openPool(t)
	if _, err := pool.Exec(context.Background(),
		`delete from settings where kind = 'budget'`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	return admin.NewBudgets(pool, settings.NewStore(pool, v))
}

var cx = domain.Scope{Company: "acme", Area: "cx"}

func TestResolve_areaCannotRaiseWhatItsCompanyAllows(t *testing.T) {
	b := newBudgets(t)
	ctx := context.Background()

	// PRD §3.1: budgets inherit downwards and never widen.
	if err := b.Put(ctx, "usr_ana", domain.ScopeBudget{
		Scope: domain.Scope{Company: "acme"}, Period: domain.PeriodMonthly,
		Budget: domain.Budget{Micros: 500_000}, Enabled: true,
	}); err != nil {
		t.Fatalf("Put company: %v", err)
	}
	if err := b.Put(ctx, "usr_ana", domain.ScopeBudget{
		Scope: cx, Period: domain.PeriodMonthly,
		Budget: domain.Budget{Micros: 9_000_000}, Enabled: true,
	}); err != nil {
		t.Fatalf("Put area: %v", err)
	}

	ceiling, _, err := b.Resolve(ctx, cx)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ceiling.Micros != 500_000 {
		t.Errorf("Micros = %d, want the company's tighter ceiling", ceiling.Micros)
	}
}

func TestResolve_areaTightensWhatItsCompanyAllows(t *testing.T) {
	b := newBudgets(t)
	ctx := context.Background()

	for _, budget := range []domain.ScopeBudget{
		{Scope: domain.Scope{Company: "acme"}, Period: domain.PeriodMonthly,
			Budget: domain.Budget{Micros: 900_000}, Enabled: true},
		{Scope: cx, Period: domain.PeriodDaily,
			Budget: domain.Budget{Micros: 100_000}, Enabled: true},
	} {
		if err := b.Put(ctx, "usr_ana", budget); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	ceiling, period, err := b.Resolve(ctx, cx)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ceiling.Micros != 100_000 {
		t.Errorf("Micros = %d, want the area's own, tighter ceiling", ceiling.Micros)
	}
	// The period comes from the level actually constraining, because that is
	// the window the figure has to be measured over.
	if period != domain.PeriodDaily {
		t.Errorf("period = %q, want the tightest level's", period)
	}
}

func TestResolve_aDisabledCeilingDoesNotConstrain(t *testing.T) {
	b := newBudgets(t)
	ctx := context.Background()

	if err := b.Put(ctx, "usr_ana", domain.ScopeBudget{
		Scope: cx, Period: domain.PeriodMonthly,
		Budget: domain.Budget{Micros: 1}, Enabled: false,
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Turning a ceiling off is the switch an operator reaches for at 3am; it
	// must not need the row deleted, and it must actually stop constraining.
	ceiling, _, err := b.Resolve(ctx, cx)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ceiling.Micros != 0 {
		t.Errorf("Micros = %d, want no ceiling from a disabled budget", ceiling.Micros)
	}
}

func TestResolve_installationCeilingReachesEveryArea(t *testing.T) {
	b := newBudgets(t)
	ctx := context.Background()

	if err := b.Put(ctx, "usr_ana", domain.ScopeBudget{
		Period: domain.PeriodMonthly, Budget: domain.Budget{Micros: 250_000}, Enabled: true,
	}); err != nil {
		t.Fatalf("Put installation: %v", err)
	}

	ceiling, _, err := b.Resolve(ctx, domain.Scope{Company: "outra", Area: "financeiro"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ceiling.Micros != 250_000 {
		t.Errorf("Micros = %d, want the installation ceiling", ceiling.Micros)
	}
}

func TestPut_periodMustBeOneWeCanMeasure(t *testing.T) {
	b := newBudgets(t)

	// A window nobody can compute is a ceiling nobody can enforce.
	err := b.Put(context.Background(), "usr_ana", domain.ScopeBudget{
		Scope: cx, Period: "sempre", Budget: domain.Budget{Micros: 1}, Enabled: true,
	})
	if err == nil {
		t.Fatal("Put accepted a period the platform cannot measure")
	}
}

func TestPut_isRecordedWithWhoSetIt(t *testing.T) {
	b := newBudgets(t)
	pool := openPool(t)
	ctx := context.Background()

	if err := b.Put(ctx, "usr_ana", domain.ScopeBudget{
		Scope: cx, Period: domain.PeriodMonthly,
		Budget: domain.Budget{Micros: 500_000}, Enabled: true,
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Raising a ceiling is exactly the change an auditor asks about after an
	// expensive month.
	var action, by string
	if err := pool.QueryRow(ctx,
		`select action, principal_id from admin_events where target = 'acme/cx' order by event_id desc limit 1`,
	).Scan(&action, &by); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if action != "budget.changed" || by != "usr_ana" {
		t.Errorf("event = %s by %s, want budget.changed by usr_ana", action, by)
	}
}
