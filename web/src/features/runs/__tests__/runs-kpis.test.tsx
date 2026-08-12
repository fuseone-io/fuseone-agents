import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { RunsKpis } from "@/features/runs/runs-kpis";
import type { RunStats } from "@/lib/api/client";

const stats = (over: Partial<RunStats> = {}): RunStats => ({
  total: 0,
  ended: 0,
  byPhase: {},
  ...over,
});

describe("the run figures", () => {
  it("states a share with what it is a share of", () => {
    render(
      <RunsKpis
        isLoading={false}
        stats={stats({ total: 4, byPhase: { finished: 3 } })}
      />,
    );

    expect(screen.getByText("75%")).toBeInTheDocument();
    expect(screen.getByText("3 de 4")).toBeInTheDocument();
  });

  it("states no more precision than the sample carries", () => {
    // A tenth of a point on four runs is noise, and in a monospaced face the
    // decimal comma takes a full cell and reads as a gap.
    render(
      <RunsKpis
        isLoading={false}
        stats={stats({ total: 3, byPhase: { finished: 1 } })}
      />,
    );
    expect(screen.getByText("33%")).toBeInTheDocument();
  });

  it("refuses to print a median when nothing has finished", () => {
    // A median of 0ms reads as a measurement. There is nothing to measure.
    render(
      <RunsKpis
        isLoading={false}
        stats={stats({ total: 2, byPhase: { running: 2 } })}
      />,
    );

    expect(
      screen.getByText(/nenhuma execução concluída ainda/),
    ).toBeInTheDocument();
  });

  it("counts approvals and parked runs together as waiting on a person", () => {
    render(
      <RunsKpis
        isLoading={false}
        stats={stats({
          total: 5,
          byPhase: { awaiting_approval: 2, parked: 1, finished: 2 },
        })}
      />,
    );

    expect(screen.getByText("Esperando pessoa")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
  });
});
