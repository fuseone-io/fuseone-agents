import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ExperimentList } from "@/features/experiments/experiment-list";
import { renderWithClient, stubApi } from "@/features/experiments/__tests__/experiment-fixtures";
import type { ExperimentListing } from "@/features/experiments/experiments-api";

/*
Picking two experiments to compare. They are compared oldest to newest whatever order
they were picked in, so the same two always read the same way round.
*/

function listing(id: string, openedAt: string): ExperimentListing {
  return {
    id, openedAt, datasetVersion: 3, version: "v2a1b2c3d4e5", config: {},
    repetitions: 1, runs: 2, cases: 2,
  };
}

function showList(experiments: ExperimentListing[]) {
  stubApi({ "GET /agents/triage/experiments": { experiments } });
  const onCompare = vi.fn();
  renderWithClient(<ExperimentList agentId="triage" onOpen={vi.fn()} onCompare={onCompare} />);
  return onCompare;
}

describe("the list of experiments", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("compares the two picked, the older first, whichever was picked first", async () => {
    const onCompare = showList([
      listing("exp_new", "2026-11-10T10:00:00Z"),
      listing("exp_old", "2026-11-09T10:00:00Z"),
    ]);
    const user = userEvent.setup();

    const boxes = await screen.findAllByRole("checkbox");
    const compare = screen.getByRole("button", { name: "Comparar os dois" });
    await user.click(boxes[0]!);
    expect(compare).toBeDisabled();
    await user.click(boxes[1]!);
    await user.click(compare);

    expect(onCompare).toHaveBeenCalledWith("exp_old", "exp_new");
  });

  it("says what an experiment is when there is none", async () => {
    showList([]);

    expect(await screen.findByText(/Nenhum experimento ainda/)).toBeInTheDocument();
  });

  it("says an experiment that overrode nothing ran as published", async () => {
    showList([listing("exp_a", "2026-11-10T10:00:00Z")]);

    expect(await screen.findByText("Como publicado")).toBeInTheDocument();
  });
});
