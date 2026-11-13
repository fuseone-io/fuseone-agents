import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ExperimentCost } from "@/features/experiments/experiment-cost";
import { ExperimentStart } from "@/features/experiments/experiment-start";
import { renderWithClient, stubApi } from "@/features/experiments/__tests__/experiment-fixtures";

/*
Starting an experiment. The cost is stated before it starts and counts every repetition
as the run it is, and a form nobody changed asks for the version as published.
*/

const agent = {
  agentId: "triage", versionId: "v2a1b2c3d4e5", scope: { company: "acme", area: "cx" },
  name: "Triage", provider: "anthropic", model: "test-model", tools: [],
  budget: { micros: 500_000, steps: 20 }, publishedAt: "2026-11-01T10:00:00Z", latest: true,
};

function showStart() {
  const posted = stubApi({
    "GET /agents/triage": { agent, versions: [{ versionId: agent.versionId, publishedAt: agent.publishedAt, latest: true }] },
    "GET /agents/triage/regressions": { items: [{ id: "a" }, { id: "b" }, { id: "c" }] },
    "GET /providers?scope=acme%2Fcx": { items: [] },
    "POST /agents/triage/experiments": { id: "exp_triage_1", cases: 3, repetitions: 1, runs: 3, datasetVersion: 2 },
  });
  const onStarted = vi.fn();
  renderWithClient(<ExperimentStart agentId="triage" onClose={vi.fn()} onStarted={onStarted} />);
  return { posted, onStarted };
}

describe("starting an experiment", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("states what it may cost, every case counted once per repetition", async () => {
    showStart();

    expect(await screen.findByText(/Casos: 3 × repetições: 1 = 3 runs/)).toBeInTheDocument();
  });

  it("asks for the version as published when nothing was changed", async () => {
    const { posted, onStarted } = showStart();
    const user = userEvent.setup();

    await screen.findByText(/Casos: 3/);
    await user.click(screen.getByRole("button", { name: "Iniciar experimento" }));

    await waitFor(() => expect(onStarted).toHaveBeenCalledWith("exp_triage_1"));
    expect(posted[0]?.body).toEqual({ version: agent.versionId, repetitions: 1 });
  });

  it("counts each repetition as its own run", async () => {
    stubApi({ "GET /agents/triage/regressions": { items: [{ id: "a" }, { id: "b" }, { id: "c" }] } });
    renderWithClient(<ExperimentCost agentId="triage" agent={agent} repetitions={3} />);

    expect(
      await screen.findByText(/Casos: 3 × repetições: 3 = 9 runs, até .* cada: até .* no total/),
    ).toBeInTheDocument();
  });
});
