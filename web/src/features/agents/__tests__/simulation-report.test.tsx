import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { SimulationReportView } from "@/features/agents/simulation-report";
import { setLocale } from "@/i18n";
import type { SimulationCase, SimulationReport } from "@/features/agents/simulation-api";

vi.mock("@/features/agents/version-comparison", () => ({
  VersionComparison: () => null,
}));

type SimulationQuery = Parameters<typeof SimulationReportView>[0]["report"];

function renderReport(cases: SimulationCase[]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const report = {
    data: { id: "sim-1", cases, running: false } satisfies SimulationReport,
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  };
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <SimulationReportView agentId="triage" report={report as unknown as SimulationQuery} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function answeredCase(over: Partial<SimulationCase> = {}): SimulationCase {
  return {
    runId: "run-1",
    settled: "finished",
    steps: 8,
    cost: { micros: 7600 },
    outcome: "Answered.",
    acted: [],
    ...over,
  };
}

describe("simulation report", () => {
  beforeEach(() => {
    setLocale("en-US");
    vi.restoreAllMocks();
  });
  afterEach(() => vi.unstubAllGlobals());

  it("saves a clean rehearsal as a regression baseline", async () => {
    const posted: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | RequestInfo | URL, init?: RequestInit) => {
        const request = input instanceof Request ? input : new Request(input, init);
        posted.push(JSON.parse(await request.text()));
        return new Response(JSON.stringify({ id: "reg-1" }), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );

    renderReport([answeredCase()]);

    expect(screen.getByRole("button", { name: "Correct" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Save case" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toEqual({
      runId: "run-1",
      expectations: [{ kind: "settles", value: "finished" }],
    });
  });

  it("keeps correction as the exit for a rehearsal that needs review", () => {
    renderReport([answeredCase({ reason: "attempts_exhausted" })]);

    expect(screen.getByRole("button", { name: "Correct" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save case" })).not.toBeInTheDocument();
  });

  it("does not offer to save a corpus case again", () => {
    renderReport([answeredCase({ id: "case-1" })]);

    expect(screen.getByRole("button", { name: "Correct" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save case" })).not.toBeInTheDocument();
  });

  it("only marks the case being saved as pending", async () => {
    let finish!: () => void;
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            finish = () =>
              resolve(
                new Response(JSON.stringify({ id: "reg-1" }), {
                  status: 201,
                  headers: { "Content-Type": "application/json" },
                }),
              );
          }),
      ),
    );

    renderReport([
      answeredCase({ runId: "run-1" }),
      answeredCase({ runId: "run-2" }),
    ]);

    const saves = screen.getAllByRole("button", { name: "Save case" });
    expect(saves).toHaveLength(2);
    const first = saves[0]!;
    const second = saves[1]!;
    await userEvent.click(first);

    await waitFor(() => expect(first).toBeDisabled());
    expect(second).not.toBeDisabled();

    finish();
  });
});
