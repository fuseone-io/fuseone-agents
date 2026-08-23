import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { CostPage } from "@/features/cost/cost-page";

const hooks = vi.hoisted(() => ({
  daily: {},
  byAgent: {},
  byArea: {},
  planningByModel: {},
  planningByAgent: {},
  refetchDaily: vi.fn(),
  refetchPlanningModel: vi.fn(),
  refetchPlanningAgent: vi.fn(),
}));

vi.mock("@/features/cost/api", () => ({
  useCostWindow: () => ({
    from: "2026-08-20T00:00:00Z",
    to: "2026-08-21T00:00:00Z",
  }),
  useCostRollup: (_from: string, _to: string, groupBy: string) => {
    if (groupBy === "day") return hooks.daily;
    if (groupBy === "area") return hooks.byArea;
    return hooks.byAgent;
  },
  usePlanningSpend: (_from: string, _to: string, cut: string) =>
    cut === "models" ? hooks.planningByModel : hooks.planningByAgent,
}));

vi.mock("@/features/cost/budget-alerts", () => ({
  BudgetAlerts: () => null,
}));

vi.mock("@/features/cost/cost-caps", () => ({
  CostCaps: () => null,
}));

describe("cost page", () => {
  beforeEach(() => {
    hooks.refetchDaily.mockReset();
    hooks.refetchPlanningModel.mockReset();
    hooks.refetchPlanningAgent.mockReset();
    hooks.daily = {
      data: rollup("day", "2026-08-20"),
      isLoading: false,
      error: null,
      refetch: hooks.refetchDaily,
    };
    hooks.byAgent = {
      data: rollup("agent", "triage-agent"),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    };
    hooks.byArea = {
      data: rollup("area", "platform"),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    };
    hooks.planningByModel = {
      data: undefined,
      isLoading: false,
      error: new Error("planning projection is down"),
      refetch: hooks.refetchPlanningModel,
    };
    hooks.planningByAgent = {
      data: undefined,
      isLoading: false,
      error: null,
      refetch: hooks.refetchPlanningAgent,
    };
  });

  it("keeps the cost page usable when the planning projection fails", async () => {
    const user = userEvent.setup();

    render(<CostPage />);

    expect(screen.getByText("triage-agent")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Tentar de novo" }));

    expect(hooks.refetchDaily).not.toHaveBeenCalled();
    expect(hooks.refetchPlanningModel).toHaveBeenCalledOnce();
    expect(hooks.refetchPlanningAgent).toHaveBeenCalledOnce();
  });
});

function rollup(groupBy: "agent" | "area" | "day", key: string) {
  return {
    from: "2026-08-20T00:00:00Z",
    to: "2026-08-21T00:00:00Z",
    groupBy,
    total: { micros: 1200, inputTokens: 1000 },
    buckets: [
      {
        key,
        runs: 2,
        cost: { micros: 1200, inputTokens: 1000 },
      },
    ],
  };
}
