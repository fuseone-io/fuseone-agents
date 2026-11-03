import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { expect, vi } from "vitest";
import { WorkItemPage } from "@/features/work-items/work-item-page";
import type {
  Agent,
  WorkItemMeasureRow,
  WorkItemMeasures,
} from "@/lib/api/client";

export const PAST = "2026-10-26T00:00:00Z";
export const NOW = "2026-11-02T00:00:00Z";

export const row = (
  over: Partial<WorkItemMeasureRow> = {},
  proposals: Partial<WorkItemMeasureRow["proposals"]> = {},
): WorkItemMeasureRow => ({
  week: PAST,
  complete: true,
  agentId: "developer",
  runs: 4,
  openRuns: 1,
  cards: 2,
  proposals: {
    requested: 6,
    approved: 3,
    refused: 1,
    abandoned: 0,
    pending: 2,
    revisedAfterRefusal: 1,
    repeatedAfterRefusal: 0,
    standing: 0,
    ...proposals,
  },
  costOnCardsMicros: 3_000_000,
  costInheritedMicros: 500_000,
  costOffCardMicros: 250_000,
  ...over,
});

export const measures = (rows: WorkItemMeasureRow[]): WorkItemMeasures => ({
  weekStart: "monday",
  timezone: "UTC",
  from: "2026-09-14T00:00:00Z",
  to: "2026-11-09T00:00:00Z",
  asOf: "2026-11-03T12:00:00Z",
  rows,
  unavailable: [
    { measure: "edited", reason: "not_recorded", detail: "not recorded" },
    { measure: "laneTime", reason: "board_only", detail: "board" },
    { measure: "caughtLater", reason: "board_only", detail: "board" },
  ],
});

/** Only what the panel reads of an agent; the rest of the shape is noise. */
export const agent = (
  agentId: string,
  company: string,
  area: string,
  standingApprovals?: Agent["standingApprovals"],
) => ({ agentId, scope: { company, area }, standingApprovals });

type AgentStub = ReturnType<typeof agent>;

/**
 * The network boundary. The agents list is narrowed the way the server
 * narrows it, by the company and area the request carries.
 */
export function serve(body: WorkItemMeasures, agents: AgentStub[] = []) {
  const asked: URL[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      const url = new URL(input.url);
      asked.push(url);
      const company = url.searchParams.get("company");
      const area = url.searchParams.get("area");
      const payload = url.pathname.endsWith("/agents")
        ? {
            items: agents.filter(
              (a) =>
                (!company || a.scope.company === company) &&
                (!area || a.scope.area === area),
            ),
          }
        : url.pathname.endsWith("/trail")
          ? { ref: url.searchParams.get("ref"), runs: [], roles: [] }
          : body;
      return new Response(JSON.stringify(payload), {
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  return asked;
}

export const measureCalls = (asked: URL[]) =>
  asked.filter((u) => u.pathname.endsWith("/work-items/measures"));

export function open(path = "/work-items") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/work-items" element={<WorkItemPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

/** The cell of `row` under the column headed `column`, in that row's table. */
export function cell(tableRow: HTMLElement, column: string | RegExp) {
  const table = tableRow.closest("table") as HTMLElement;
  const heads = within(table).getAllByRole("columnheader");
  const index = heads.findIndex((h) =>
    typeof column === "string"
      ? h.textContent === column
      : column.test(h.textContent ?? ""),
  );
  expect(index).toBeGreaterThanOrEqual(0);
  return within(tableRow).getAllByRole("cell")[index] as HTMLElement;
}

export const agentRow = async (agentId: string) =>
  screen.findByRole("row", { name: new RegExp(`^${agentId}`) });

/** Radix's Select asks for pointer capture and scrolls its options into view;
 *  jsdom has neither. */
export function stubSelectDom() {
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
}
