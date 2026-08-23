import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { PlanningSpendPanel } from "@/features/cost/planning-spend-panel";
import type { components } from "@/lib/api/schema.gen";

type PlanningSpendRollup = components["schemas"]["PlanningSpendRollup"];

describe("planning spend panel", () => {
  it("names the expensive model and flags calls without a configured rate", () => {
    render(
      <PlanningSpendPanel
        byModel={rollup("model", [
          bucket({ provider: "anthropic", model: "claude-opus-5", unpriced: 2 }),
        ])}
        byAgent={rollup("agent", [
          bucket({ agent: "troubleshooting-devops", unpriced: 2 }),
        ])}
      />,
    );

    expect(screen.getByText("claude-opus-5")).toBeInTheDocument();
    expect(screen.getByText("anthropic")).toBeInTheDocument();
    expect(screen.getByText("troubleshooting-devops")).toBeInTheDocument();
    expect(screen.getAllByText("2 sem tarifa")).toHaveLength(4);
  });

  it("says when a cut continues past the visible rows", () => {
    render(
      <PlanningSpendPanel
        byModel={rollup(
          "model",
          Array.from({ length: 9 }, (_, index) =>
            bucket({ provider: "anthropic", model: `model-${index + 1}` }),
          ),
        )}
      />,
    );

    expect(screen.getByText("model-8")).toBeInTheDocument();
    expect(screen.queryByText("model-9")).not.toBeInTheDocument();
    expect(screen.getByText("8 de 9")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Carregar mais" })).toBeEnabled();
  });

  it("only warns about projection start when the selected window is partial", () => {
    const complete = rollup("model", [bucket({ model: "complete" })]);
    complete.projectedFrom = "2026-08-19T00:00:00Z";
    const { rerender } = render(<PlanningSpendPanel byModel={complete} />);

    expect(screen.queryByText(/projetado desde/)).not.toBeInTheDocument();

    const partial = rollup("model", [bucket({ model: "partial" })]);
    partial.projectedFrom = "2026-08-20T12:00:00Z";
    rerender(<PlanningSpendPanel byModel={partial} />);

    expect(screen.getByText(/projetado desde/)).toBeInTheDocument();
  });
});

function rollup(
  groupBy: "agent" | "model",
  buckets: PlanningSpendRollup["buckets"],
): PlanningSpendRollup {
  return {
    from: "2026-08-20T00:00:00Z",
    to: "2026-08-21T00:00:00Z",
    projectedFrom: "2026-08-20T12:00:00Z",
    groupBy,
    calls: buckets.reduce((sum, b) => sum + b.calls, 0),
    unpriced: buckets.reduce((sum, b) => sum + b.unpriced, 0),
    total: { micros: 900, inputTokens: 1000 },
    buckets,
  };
}

function bucket({
  provider = "",
  model = "",
  agent = "",
  unpriced = 0,
}: {
  provider?: string;
  model?: string;
  agent?: string;
  unpriced?: number;
}): PlanningSpendRollup["buckets"][number] {
  return {
    provider,
    model,
    agent,
    calls: 3,
    runs: 2,
    unpriced,
    cost: { micros: 900, inputTokens: 1000 },
  };
}
