import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { vi } from "vitest";
import type {
  ExperimentCase,
  ExperimentComparison,
  ExperimentReport,
  ExperimentSummary,
} from "@/features/experiments/experiments-api";

export function renderWithClient(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
}

/**
 * Answers the API by path, and records what was posted. An unknown path is a
 * 404, so a screen asking for something nobody stubbed fails loudly.
 */
export function stubApi(routes: Record<string, unknown>) {
  const posted: { path: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : new Request(input, init);
      const url = new URL(request.url);
      const path = url.pathname.replace(/^\/api\/v1/, "");
      if (request.method === "POST") {
        posted.push({ path, body: await request.json() });
      }
      // The query first, so two versions of one agent can answer differently.
      const withQuery = `${request.method} ${path}${url.search}`;
      const key = withQuery in routes ? withQuery : `${request.method} ${path}`;
      const found = key in routes;
      return new Response(JSON.stringify(found ? routes[key] : { title: "not found" }), {
        status: found ? (request.method === "POST" ? 202 : 200) : 404,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  return posted;
}

export function summary(over: Partial<ExperimentSummary> = {}): ExperimentSummary {
  return {
    id: "exp_triage_1", datasetVersion: 3, version: "v2a1b2c3d4e5", config: { model: "other-model" },
    casesRun: 2, casesInVersion: 3, erased: 1, repetitions: 5,
    held: 1, broken: 1, cost: { micros: 100_000 }, running: false,
    ...over,
  };
}

export function experimentCase(id: string, brokenIn: number, ran: number): ExperimentCase {
  return {
    brokenIn, ran,
    case: {
      id, settled: "finished", steps: 3, cost: { micros: 10_000 },
      unmet: brokenIn > 0 || ran === 0 ? [{ kind: "calls", value: "crm.lookup" }] : undefined,
    },
  };
}

export function report(cases: ExperimentCase[]): ExperimentReport {
  return { summary: summary(), cases };
}

export function comparison(over: Partial<ExperimentComparison> = {}): ExperimentComparison {
  return {
    from: summary({ id: "exp_a", datasetVersion: 2 }),
    to: summary({ id: "exp_b", datasetVersion: 3 }),
    sharedOnly: true, onlyFrom: 1, onlyTo: 0, regressed: 1, fixed: 0, costMicros: 0,
    cases: [
      { id: "estorno", was: "held", now: "broke", costMicros: 0, steps: 0,
        wasBrokenIn: 0, wasRan: 5, nowBrokenIn: 2, nowRan: 5 },
      { id: "consulta", was: "held", now: "held", costMicros: 0, steps: 0,
        wasBrokenIn: 0, wasRan: 5, nowBrokenIn: 0, nowRan: 5 },
    ],
    ...over,
  };
}
