import { screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ExperimentComparisonView } from "@/features/experiments/experiment-comparison";
import {
  comparison,
  renderWithClient,
  stubApi,
} from "@/features/experiments/__tests__/experiment-fixtures";

/*
Comparing experiments (NT-015): on different dataset versions only the shared cases are
compared, and the screen says so — with what was left out of each side — before any row.
*/

function showComparison(over = {}) {
  stubApi({ "GET /agents/triage/experiment-comparison": comparison(over) });
  renderWithClient(<ExperimentComparisonView agentId="triage" from="exp_a" to="exp_b" />);
}

describe("comparing two experiments", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("says only the shared cases are compared across dataset versions, and what was left out", async () => {
    showComparison();

    expect(
      await screen.findByText(
        "Versões do dataset diferentes (2 e 3): só os 2 casos em comum são comparados. Ficaram de fora 1 só em De e 0 só em Para.",
      ),
    ).toBeInTheDocument();
  });

  it("says nothing of the kind on one dataset version", async () => {
    showComparison({ sharedOnly: false, onlyFrom: 0, onlyTo: 0 });

    await screen.findByText("estorno");
    expect(screen.queryByText(/Versões do dataset diferentes/)).not.toBeInTheDocument();
  });

  it("shows in how many repetitions a case broke on each side", async () => {
    showComparison();

    const row = (await screen.findByText("estorno")).closest("tr")!;
    expect(within(row).getByText("0 de 5 → 2 de 5")).toBeInTheDocument();
  });
});
