import { scopeLabel, scopePath } from "@/features/admin/budget-scope";
import type { ScopeBudget } from "@/features/admin/api";

export interface Spend {
  byCompany: Map<string, number>;
  byArea: Map<string, number>;
  total: number;
}

export interface CeilingRow {
  key: string;
  label: string;
  cap: number;
  spent: number;
}

/**
 * Pairs each ceiling with what was actually spent under it.
 *
 * The dimension has to match the ceiling's own: a company ceiling read against
 * an area rollup finds nothing and reports the company as untouched. This read
 * every ceiling as an area one, so three company ceilings all looked up the
 * empty area, all showed zero spent, and all carried the same React key —
 * which is how the bug announced itself before anybody noticed the figures.
 */
export function ceilingRows(
  budgets: ScopeBudget[],
  spend: Spend,
): CeilingRow[] {
  return budgets.map((budget) => ({
    key: `${budget.scopeKind}:${scopePath(budget)}`,
    label: scopeLabel(budget),
    cap: budget.micros ?? 0,
    spent: spentUnder(budget, spend),
  }));
}

function spentUnder(budget: ScopeBudget, spend: Spend): number {
  switch (budget.scopeKind) {
    case "installation":
      return spend.total;
    case "company":
      return spend.byCompany.get(budget.scope?.company ?? "") ?? 0;
    default:
      return spend.byArea.get(budget.scope?.area ?? "") ?? 0;
  }
}
