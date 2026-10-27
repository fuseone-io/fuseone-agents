import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { BudgetAlerts } from "@/features/cost/budget-alerts";
import type { BudgetAlert } from "@/features/cost/alerts-api";

const hooks = vi.hoisted(() => ({ alerts: [] as BudgetAlert[] }));

vi.mock("@/features/cost/alerts-api", () => ({
  useBudgetAlerts: () => ({ data: hooks.alerts }),
}));

function alert(partial: Partial<BudgetAlert>): BudgetAlert {
  return {
    scope: { company: "acme", area: "ops" },
    threshold: 100,
    spentMicros: 100_062_552,
    ceilingMicros: 100_000_000,
    since: "2026-10-27T00:00:00Z",
    period: "daily",
    ...partial,
  };
}

describe("budget alerts", () => {
  // A daily budget crossed must not announce itself as a monthly one: the
  // reader would go hunting for a monthly ceiling that does not exist (#146).
  it("names today's budget when the crossed ceiling is daily", () => {
    hooks.alerts = [alert({ period: "daily" })];
    render(<BudgetAlerts />);
    expect(
      screen.getByText("ops esgotou o orçamento de hoje"),
    ).toBeInTheDocument();
  });

  it("names the month's budget when the crossed ceiling is monthly", () => {
    hooks.alerts = [
      alert({ period: "monthly", threshold: 80, spentMicros: 85_000_000 }),
    ];
    render(<BudgetAlerts />);
    expect(
      screen.getByText("ops passou de 80% do orçamento do mês"),
    ).toBeInTheDocument();
  });
});
