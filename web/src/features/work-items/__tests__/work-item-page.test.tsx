import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkItemPage } from "@/features/work-items/work-item-page";
import { workItemPath } from "@/features/work-items/work-item-path";
import type { WorkItemTrail } from "@/lib/api/client";

const REF = "github:fuseone-io/fuseone-agents#204";

const run = (runId: string, agentId: string, claimedAt: string) => ({
  runId,
  agentId,
  scope: { company: "acme", area: "factory" },
  phase: "finished" as const,
  startedAt: claimedAt,
  endedAt: claimedAt,
  claimedAt,
  link: "claim" as const,
});

const firstPage: WorkItemTrail = {
  ref: REF,
  runs: [run("run-triage", "triage", "2026-10-01T09:00:00Z")],
  roles: [
    {
      agentId: "triage",
      runs: 1,
      approvalsRequested: 3,
      approved: 2,
      refused: 1,
      abandoned: 0,
      firstClaimedAt: "2026-10-01T09:00:00Z",
      costMicros: 1_500_000,
    },
    {
      agentId: "developer",
      runs: 2,
      approvalsRequested: 0,
      approved: 0,
      refused: 0,
      abandoned: 1,
      firstClaimedAt: "2026-10-01T10:00:00Z",
    },
  ],
  nextCursor: "next-1",
};

function serve(pages: Record<string, WorkItemTrail | number>) {
  const asked: URL[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      const url = new URL(input.url);
      asked.push(url);
      const body = pages[url.searchParams.get("cursor") ?? ""];
      if (typeof body === "number") return new Response("{}", { status: body });
      return new Response(JSON.stringify(body), {
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  return asked;
}

function open(path = workItemPath(REF)) {
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

describe("a work item's trail", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("asks for the reference and says the view is of this reference only", async () => {
    const asked = serve({ "": firstPage });
    open();

    expect(await screen.findByText(REF)).toBeInTheDocument();
    expect(screen.getByText(/somente desta referência/i)).toBeInTheDocument();
    expect(screen.getByText(/issue e o pull request/i)).toBeInTheDocument();
    expect(asked[0]?.searchParams.get("ref")).toBe(REF);
  });

  it("shows each role's approvals, and its cost only when the server sent one", async () => {
    serve({ "": firstPage });
    open();

    const triage = await screen.findByRole("row", { name: /^triage/ });
    expect(within(triage).getByText("3")).toBeInTheDocument();
    expect(within(triage).getByText(/R\$\s?1,50/)).toBeInTheDocument();
    const developer = screen.getByRole("row", { name: /^developer/ });
    expect(within(developer).queryByText(/R\$/)).not.toBeInTheDocument();
    expect(
      within(developer).getByText(/custo não visível/i),
    ).toBeInTheDocument();
  });

  it("links each run to its own screen and pages on with the server's cursor", async () => {
    const asked = serve({
      "": firstPage,
      "next-1": {
        ref: REF,
        runs: [run("run-dev", "developer", "2026-10-01T10:00:00Z")],
      },
    });
    open();

    const link = await screen.findByRole("link", { name: "run-triage" });
    expect(link).toHaveAttribute("href", "/runs/run-triage");
    await userEvent.click(
      screen.getByRole("button", { name: /carregar mais/i }),
    );

    expect(
      await screen.findByRole("link", { name: "run-dev" }),
    ).toHaveAttribute("href", "/runs/run-dev");
    expect(asked[1]?.searchParams.get("cursor")).toBe("next-1");
    expect(screen.getAllByRole("row", { name: /^triage/ })).toHaveLength(1);
  });

  it("says no run you can see claimed it when the trail is empty", async () => {
    serve({ "": { ref: REF, runs: [], roles: [] } });
    open();

    expect(
      await screen.findByText(/nenhuma execução que você pode ver/i),
    ).toBeInTheDocument();
  });

  it("offers a retry when the trail cannot be read", async () => {
    serve({ "": 500 });
    open();

    expect(
      await screen.findByRole("button", { name: /tentar de novo/i }),
    ).toBeInTheDocument();
  });

  it("asks nothing of the server without a reference", () => {
    const asked = serve({});
    open("/work-items");

    expect(screen.getByText(/nenhuma referência/i)).toBeInTheDocument();
    expect(asked).toHaveLength(0);
  });
});

describe("the path to a work item", () => {
  it("encodes the reference so the issue number survives the URL", () => {
    expect(workItemPath(REF)).toBe(
      "/work-items?ref=github%3Afuseone-io%2Ffuseone-agents%23204",
    );
  });
});
