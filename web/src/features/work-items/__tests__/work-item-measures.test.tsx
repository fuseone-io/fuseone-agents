import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WorkItemPage } from "@/features/work-items/work-item-page";
import { workItemPath } from "@/features/work-items/work-item-path";
import { i18next } from "@/i18n";
import type {
  WorkItemMeasureRow,
  WorkItemMeasures,
} from "@/lib/api/client";

const PAST = "2026-10-26T00:00:00Z";
const NOW = "2026-11-02T00:00:00Z";

const row = (
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

const UNAVAILABLE: WorkItemMeasures["unavailable"] = [
  { measure: "edited", reason: "not_recorded", detail: "not recorded" },
  { measure: "laneTime", reason: "board_only", detail: "board" },
  { measure: "caughtLater", reason: "board_only", detail: "board" },
];

const measures = (rows: WorkItemMeasureRow[]): WorkItemMeasures => ({
  weekStart: "monday",
  timezone: "UTC",
  from: "2026-09-14T00:00:00Z",
  to: "2026-11-09T00:00:00Z",
  asOf: "2026-11-03T12:00:00Z",
  rows,
  unavailable: UNAVAILABLE,
});

function serve(body: WorkItemMeasures) {
  const asked: URL[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      const url = new URL(input.url);
      asked.push(url);
      const payload = url.pathname.endsWith("/agents")
        ? { items: [] }
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

function open(path = "/work-items") {
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
function cell(tableRow: HTMLElement, column: string | RegExp) {
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

const agentRow = async (agent: string) =>
  screen.findByRole("row", { name: new RegExp(`^${agent}`) });

describe("the weekly panel by role", () => {
  beforeEach(() => {
    // Radix's Select asks for pointer capture and scrolls its options into
    // view; jsdom has neither.
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.scrollIntoView ??= () => {};
  });
  afterEach(() => vi.unstubAllGlobals());

  it("asks for eight weeks by default and shows a week per table", async () => {
    const asked = serve(measures([row()]));
    open();

    expect(await agentRow("developer")).toBeInTheDocument();
    const call = asked.find((u) => u.pathname.endsWith("/work-items/measures"));
    expect(call?.searchParams.get("weeks")).toBe("8");
    expect(
      screen.getByRole("heading", { name: "Por papel, por semana" }),
    ).toBeInTheDocument();
  });

  it("asks again for the weeks the reader picks", async () => {
    const asked = serve(measures([row()]));
    open();
    await agentRow("developer");

    await userEvent.click(screen.getByRole("combobox", { name: /semanas/i }));
    await userEvent.click(screen.getByRole("option", { name: /26 semanas/i }));

    await vi.waitFor(() =>
      expect(asked.at(-1)?.searchParams.get("weeks")).toBe("26"),
    );
  });

  it("says each unavailable measure in words, never as a number", async () => {
    serve(measures([row()]));
    open();
    await agentRow("developer");

    const note = screen.getByRole("note", { name: /fora destas contas/i });
    expect(note).toHaveTextContent(
      "Edição de propostas: sem registro na plataforma",
    );
    expect(note).toHaveTextContent(
      "Tempo em lane e achados posteriores: vêm do quadro (CLI da fábrica)",
    );
    expect(note.textContent).not.toMatch(/\d/);
  });

  it("names a measure it does not know without breaking", async () => {
    serve({
      ...measures([row()]),
      // A value this console was built before: the type cannot know it.
      unavailable: [
        { measure: "newThing", reason: "other", detail: "x" },
      ] as unknown as WorkItemMeasures["unavailable"],
    });
    open();
    await agentRow("developer");

    expect(screen.getByRole("note")).toHaveTextContent(
      "Outra medida (newThing): indisponível",
    );
  });

  it("shows an absent cost as a dash, never as R$ 0", async () => {
    const { costOnCardsMicros, costInheritedMicros, costOffCardMicros, ...bare } =
      row();
    void [costOnCardsMicros, costInheritedMicros, costOffCardMicros];
    serve(measures([bare]));
    open();

    const dev = await agentRow("developer");
    expect(within(dev).queryByText(/R\$/)).toBeNull();
    expect(within(dev).getAllByText(/custo não visível/i)).toHaveLength(4);
    expect(cell(dev, "Por item")).toHaveTextContent("—");
  });

  it("shows cost per card as the cost on cards over the cards", async () => {
    serve(measures([row({ cards: 4, costOnCardsMicros: 6_000_000 })]));
    open();

    const dev = await agentRow("developer");
    expect(cell(dev, "Por item")).toHaveTextContent(/R\$\s?1,50/);
    expect(cell(dev, "Em itens")).toHaveTextContent(/R\$\s?6,00/);
  });

  it("shows cost per card as a dash when no card was claimed", async () => {
    serve(measures([row({ cards: 0, costOnCardsMicros: 0 })]));
    open();

    const dev = await agentRow("developer");
    expect(cell(dev, "Por item")).toHaveTextContent("—");
    expect(cell(dev, "Por item").textContent).not.toMatch(/R\$|NaN|∞/);
  });

  it("puts each rate beside its count over the decided proposals", async () => {
    serve(measures([row({}, { approved: 3, refused: 1 })]));
    open();

    const dev = await agentRow("developer");
    expect(cell(dev, "Aprovadas")).toHaveTextContent(/^3\s*75\s?%$/);
    expect(cell(dev, "Recusadas")).toHaveTextContent(/^1\s*25\s?%$/);
    expect(cell(dev, /^Revisadas/)).toHaveTextContent(/^1\s*100\s?%$/);
    expect(cell(dev, /^Repetidas/)).toHaveTextContent(/^0\s*0\s?%$/);
  });

  it("shows a dash, not NaN or 0 %, when nothing was decided or refused", async () => {
    serve(measures([row({}, { approved: 0, refused: 0, revisedAfterRefusal: 0 })]));
    open();

    const dev = await agentRow("developer");
    for (const column of ["Aprovadas", "Recusadas", /^Revisadas/, /^Repetidas/]) {
      expect(cell(dev, column)).toHaveTextContent(/^0\s*—/);
      expect(cell(dev, column).textContent).not.toMatch(/NaN|%/);
    }
  });

  it("warns on a row with a standing approval, which the pilot forbids", async () => {
    serve(
      measures([
        row({ agentId: "reviewer" }, { standing: 2, approved: 3 }),
        row({ agentId: "developer" }),
      ]),
    );
    open();

    const reviewer = await agentRow("reviewer");
    expect(within(reviewer).getByText(/proibida no piloto/i)).toBeInTheDocument();
    const dev = screen.getByRole("row", { name: /^developer/ });
    expect(within(dev).queryByText(/proibida no piloto/i)).toBeNull();
  });

  it("marks the current week partial and lists the newest week first", async () => {
    serve(
      measures([
        row({ week: PAST, agentId: "triage" }),
        row({ week: NOW, complete: false, agentId: "developer" }),
      ]),
    );
    open();
    await agentRow("developer");

    const weeks = screen.getAllByRole("region", { name: /semana de/i });
    expect(weeks).toHaveLength(2);
    expect(within(weeks[0]!).getByText("parcial")).toBeInTheDocument();
    expect(within(weeks[0]!).getByRole("row", { name: /^developer/ })).toBeVisible();
    expect(within(weeks[1]!).queryByText("parcial")).toBeNull();
  });

  it("shows open runs as a note beside the runs", async () => {
    serve(measures([row({ runs: 4, openRuns: 1 })]));
    open();

    const dev = await agentRow("developer");
    expect(cell(dev, "Execuções")).toHaveTextContent(/^4\s*1 aberta$/);
  });

  it("explains an empty window, and still says what it cannot count", async () => {
    serve(measures([]));
    open();

    expect(
      await screen.findByText(/nenhuma execução que você pode ver nestas semanas/i),
    ).toBeInTheDocument();
    expect(screen.getByRole("note")).toHaveTextContent(/Edição de propostas/);
  });

  it("is not shown when a reference is open: the trail is", async () => {
    const asked = serve(measures([row()]));
    open(workItemPath("github:fuseone-io/fuseone-agents#204"));

    await vi.waitFor(() => expect(asked.length).toBeGreaterThan(0));
    expect(screen.queryByRole("heading", { name: "Por papel, por semana" })).toBeNull();
    expect(asked.some((u) => u.pathname.endsWith("/measures"))).toBe(false);
  });

  it("reads in English with the same meaning", async () => {
    await i18next.changeLanguage("en-US");
    try {
      serve(measures([row({ week: NOW, complete: false })]));
      open();
      await agentRow("developer");
      expect(screen.getByRole("heading", { name: "Per role, per week" })).toBeInTheDocument();
      expect(screen.getByText("partial")).toBeInTheDocument();
      expect(screen.getByRole("note")).toHaveTextContent(
        "Lane time and caught later: come from the board (factory CLI)",
      );
    } finally {
      await i18next.changeLanguage("pt-BR");
    }
  });
});
