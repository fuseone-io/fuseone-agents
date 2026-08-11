package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

var ErrBadPeriod = errors.New("admin: a budget needs a period of daily or monthly")

// budgetName is the settings row a scope's ceiling is stored as. One per
// scope: two ceilings over different windows on the same scope is a question
// nobody has asked yet, and answering it early would double the arithmetic
// every enforcement path has to do.
const budgetName = "default"

type storedBudget struct {
	Period      string `json:"period"`
	Micros      int64  `json:"micros,omitempty"`
	Tokens      int64  `json:"tokens,omitempty"`
	ToolCalls   int64  `json:"toolCalls,omitempty"`
	Steps       int64  `json:"steps,omitempty"`
	WallClockMS int64  `json:"wallClockMs,omitempty"`
}

// Budgets configures what a scope may spend.
//
// Separate from the per-run ceiling in an agent's specification: this is a
// limit over time, and the two are combined by narrowing rather than by one
// replacing the other (PRD FO-02).
type Budgets struct {
	pool     *pgxpool.Pool
	settings *settings.Store
}

func NewBudgets(pool *pgxpool.Pool, store *settings.Store) *Budgets {
	return &Budgets{pool: pool, settings: store}
}

func (b *Budgets) List(ctx context.Context) ([]domain.ScopeBudget, error) {
	rows, err := b.settings.List(ctx, settings.KindBudget)
	if err != nil {
		return nil, err
	}

	out := make([]domain.ScopeBudget, 0, len(rows))
	for _, row := range rows {
		budget, err := decodeBudget(row)
		if err != nil {
			return nil, err
		}
		out = append(out, budget)
	}
	return out, nil
}

// Put records a ceiling and who set it.
func (b *Budgets) Put(ctx context.Context, by domain.UserID, budget domain.ScopeBudget) error {
	if !budget.Period.Valid() {
		return fmt.Errorf("%w: %q", ErrBadPeriod, budget.Period)
	}
	kind, err := scopeKindOf(budget)
	if err != nil {
		return err
	}

	value, err := json.Marshal(storedBudget{
		Period: string(budget.Period), Micros: budget.Budget.Micros,
		Tokens: budget.Budget.Tokens, ToolCalls: budget.Budget.ToolCalls,
		Steps: budget.Budget.Steps, WallClockMS: budget.Budget.WallClockMS,
	})
	if err != nil {
		return fmt.Errorf("admin: encode budget: %w", err)
	}

	return b.write(ctx, by, budget.Scope, settings.Setting{
		ScopeKind: kind, Scope: budget.Scope,
		Kind: settings.KindBudget, Name: budgetName,
		Value: value, Enabled: budget.Enabled, UpdatedBy: string(by),
	}, "budget.changed", target(budget.Scope), map[string]any{
		"period": budget.Period, "micros": budget.Budget.Micros,
		"steps": budget.Budget.Steps, "enabled": budget.Enabled,
	})
}

func (b *Budgets) Delete(ctx context.Context, by domain.UserID, scope domain.Scope) error {
	kind, err := scopeKindOf(domain.ScopeBudget{Scope: scope})
	if err != nil {
		return err
	}
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := b.settings.DeleteTx(ctx, tx, kind, scope, settings.KindBudget, budgetName); err != nil {
		return err
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: scope, Action: "budget.removed", Target: target(scope),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Resolve returns the ceiling in force for a scope, and the window it covers.
//
// It walks outward — area, then company, then installation — narrowing at each
// step, because a ceiling inherits downwards and never widens: an area cannot
// raise what its company allows (PRD §3.1). The period comes from the tightest
// level that set one, since that is the ceiling doing the constraining.
func (b *Budgets) Resolve(ctx context.Context, scope domain.Scope) (domain.Budget, domain.Period, error) {
	all, err := b.List(ctx)
	if err != nil {
		return domain.Budget{}, "", err
	}

	var (
		ceiling domain.Budget
		period  domain.Period
	)
	for _, level := range levelsOf(scope) {
		for _, candidate := range all {
			if !candidate.Enabled || candidate.Scope != level {
				continue
			}
			ceiling = ceiling.Narrow(candidate.Budget)
			if period == "" {
				period = candidate.Period
			}
		}
	}
	return ceiling, period, nil
}

// levelsOf lists the scopes that bind a run, tightest first.
func levelsOf(scope domain.Scope) []domain.Scope {
	levels := []domain.Scope{{}}
	if scope.Company != "" {
		levels = append(levels, domain.Scope{Company: scope.Company})
		if scope.Area != "" {
			levels = append(levels, scope)
		}
	}
	// Tightest first so the period comes from the level actually constraining.
	for i, j := 0, len(levels)-1; i < j; i, j = i+1, j-1 {
		levels[i], levels[j] = levels[j], levels[i]
	}
	return levels
}

func scopeKindOf(b domain.ScopeBudget) (settings.ScopeKind, error) {
	switch {
	case b.Scope.Company == "" && b.Scope.Area == "":
		return settings.ScopeInstallation, nil
	case b.Scope.Area == "":
		return settings.ScopeCompany, nil
	case b.Scope.Company != "":
		return settings.ScopeArea, nil
	}
	return "", errors.New("admin: an area budget needs the company it belongs to")
}

func decodeBudget(row settings.Setting) (domain.ScopeBudget, error) {
	var stored storedBudget
	if err := json.Unmarshal(row.Value, &stored); err != nil {
		return domain.ScopeBudget{}, fmt.Errorf("admin: decode budget for %s: %w", row.Scope, err)
	}
	return domain.ScopeBudget{
		ScopeKind: string(row.ScopeKind), Scope: row.Scope,
		Period: domain.Period(stored.Period),
		Budget: domain.Budget{
			Micros: stored.Micros, Tokens: stored.Tokens, ToolCalls: stored.ToolCalls,
			Steps: stored.Steps, WallClockMS: stored.WallClockMS,
		},
		Enabled:   row.Enabled,
		UpdatedBy: domain.UserID(row.UpdatedBy), UpdatedAt: row.UpdatedAt,
	}, nil
}

func target(scope domain.Scope) string {
	if scope.Company == "" {
		return "installation"
	}
	if scope.Area == "" {
		return string(scope.Company)
	}
	return scope.String()
}

func (b *Budgets) write(
	ctx context.Context, by domain.UserID, scope domain.Scope,
	set settings.Setting, action, target string, detail any,
) error {
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := b.settings.PutTx(ctx, tx, set); err != nil {
		return err
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: scope, Action: action, Target: target, Detail: detail,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
