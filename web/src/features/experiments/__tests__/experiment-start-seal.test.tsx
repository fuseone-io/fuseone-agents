import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ExperimentStart } from "@/features/experiments/experiment-start";
import { ExperimentsPage } from "@/features/experiments/experiments-page";
import { renderWithClient, stubApi } from "@/features/experiments/__tests__/experiment-fixtures";

/*
An experiment on a candidate (NT-018 phase 2, release 2d): it runs the seal's bytes, named
by the seal alone, never a published version, and it can start for an agent that was never
published, since the seal is all its runs need.
*/

const agent = {
  agentId: "triage", versionId: "v2a1b2c3d4e5", scope: { company: "acme", area: "cx" },
  name: "Triage", provider: "anthropic", model: "test-model", tools: [],
  budget: { micros: 500_000, steps: 20 }, publishedAt: "2026-11-01T10:00:00Z", latest: true,
};
const seal = { id: "s-1", version: "v9candidate1" };
const started = { id: "exp_triage_1", cases: 3, repetitions: 1, runs: 3, datasetVersion: 2 };

function showStart(published: boolean) {
  const posted = stubApi({
    ...(published && { "GET /agents/triage": { agent, versions: [{ versionId: agent.versionId, publishedAt: agent.publishedAt, latest: true }] } }),
    "GET /agents/triage/regressions": { items: [{ id: "a" }, { id: "b" }, { id: "c" }] },
    "GET /providers?scope=acme%2Fcx": { items: [] },
    "POST /agents/triage/experiments": started,
  });
  const onStarted = vi.fn();
  renderWithClient(<ExperimentStart agentId="triage" seal={seal} onClose={vi.fn()} onStarted={onStarted} />);
  return { posted, onStarted };
}

describe("starting an experiment on a candidate", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("names the candidate it runs, and offers no published version to choose", async () => {
    showStart(true);
    expect(await screen.findByText("Candidata v9candidate1")).toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Versão do agente" })).not.toBeInTheDocument();
  });

  // Its runs are held to the ceiling its sealed text declares, not the published version's.
  it("states the cost at the candidate's own ceiling, not the published one", async () => {
    showStart(true);
    expect(await screen.findByText(/= 3 runs, cada um até o teto que o texto selado da candidata declara/)).toBeInTheDocument();
    expect(screen.queryByText(/R\$/)).not.toBeInTheDocument();
  });

  it("asks for the seal, never a version", async () => {
    const { posted, onStarted } = showStart(true);
    await userEvent.click(await screen.findByRole("button", { name: "Iniciar experimento" }));
    await waitFor(() => expect(onStarted).toHaveBeenCalledWith("exp_triage_1"));
    expect(posted[0]?.body).toEqual({ sealId: "s-1", repetitions: 1 });
  });

  it("starts for an agent never published", async () => {
    const { posted, onStarted } = showStart(false);
    await screen.findByText("Candidata v9candidate1");
    await waitFor(() => expect(screen.getByRole("button", { name: "Iniciar experimento" })).toBeEnabled());
    await userEvent.click(screen.getByRole("button", { name: "Iniciar experimento" }));
    await waitFor(() => expect(onStarted).toHaveBeenCalled());
    expect(posted[0]?.body).toEqual({ sealId: "s-1", repetitions: 1 });
  });

  // The workbench's candidate sheet links here with the seal in the URL, so a refresh, or
  // the link sent to someone, opens the same form on the same candidate.
  it("opens the form on the candidate the URL names", async () => {
    stubApi({ "GET /agents/triage/regressions": { items: [] } });
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <MemoryRouter initialEntries={["/agents/triage/experiments?new=1&seal=s-2&candidate=v9fromurl"]}>
          <Routes><Route path="/agents/:agentId/experiments" element={<ExperimentsPage />} /></Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(await screen.findByText("Candidata v9fromurl")).toBeInTheDocument();
  });
});
