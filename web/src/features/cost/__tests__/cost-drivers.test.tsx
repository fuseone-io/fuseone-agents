import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { CostDrivers } from "@/features/cost/cost-drivers";
import { BarChart } from "@/components/shared/bar-chart";

describe("cost drivers", () => {
  it("splits the token kinds, which is why the contract separates them", () => {
    // A cache read costs a fraction of an input token: a total alone bills an
    // agent without explaining it.
    render(
      <CostDrivers
        total={{
          micros: 100,
          inputTokens: 600,
          outputTokens: 200,
          cacheReadTokens: 200,
        }}
      />,
    );

    expect(screen.getByText("Entrada")).toBeInTheDocument();
    expect(screen.getByText("Leitura de cache")).toBeInTheDocument();
    expect(screen.getByText(/60%/)).toBeInTheDocument();
  });

  it("says nothing was counted rather than drawing empty bars", () => {
    render(<CostDrivers total={{ micros: 0 }} />);
    expect(screen.getByText(/Nenhum token contabilizado/)).toBeInTheDocument();
  });
});

describe("the spend chart", () => {
  it("reads as a sentence to anyone who cannot see the bars", () => {
    render(
      <BarChart
        label="Gasto por dia"
        bars={[
          { label: "2026-08-10", value: 120, display: "R$ 0,12" },
          { label: "2026-08-11", value: 300, display: "R$ 0,30" },
        ]}
      />,
    );

    expect(
      screen.getByRole("img", { name: "Gasto por dia" }),
    ).toBeInTheDocument();
    // The caption carries every value as text; the per-bar titles are the
    // pointer tooltip and are not announced, since the aria-label wins.
    const caption = screen.getByText(
      /2026-08-10: R\$ 0,12, 2026-08-11: R\$ 0,30/,
    );
    expect(caption.tagName.toLowerCase()).toBe("figcaption");
  });
});
