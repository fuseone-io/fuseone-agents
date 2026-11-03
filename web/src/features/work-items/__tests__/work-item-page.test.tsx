import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkItemPage } from "@/features/work-items/work-item-page";
import { workItemPath } from "@/features/work-items/work-item-path";
import { i18next } from "@/i18n";
import type { WorkItemTrail } from "@/lib/api/client";

const REF = "github:fuseone-io/fuseone-agents#204";
const PR = "github:fuseone-io/fuseone-agents#240";

const run = (runId: string, agentId: string, claimedAt: string) => ({
  runId,
  agentId,
  scope: { company: "acme", area: "factory" },
  phase: "finished" as const,
  startedAt: claimedAt,
  endedAt: claimedAt,
  claimedAt,
  link: "claim" as const,
  ref: REF,
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

// A trail whose one run is there by a GitHub delivery: it claimed nothing.
const inheritedOnly: WorkItemTrail = {
  ref: REF,
  runs: [
    {
      ...run("run-hook", "triage", "2026-10-01T10:00:00Z"),
      link: "github_delivery",
    },
  ],
  roles: [
    {
      agentId: "triage",
      runs: 1,
      approvalsRequested: 0,
      approved: 0,
      refused: 0,
      abandoned: 0,
      firstClaimedAt: "2026-10-01T10:00:00Z",
      costMicros: 0,
    },
  ],
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

  it("asks for the reference and says the issue and its linked pull requests are joined", async () => {
    const asked = serve({ "": firstPage });
    open();

    expect(await screen.findByText(REF)).toBeInTheDocument();
    expect(
      screen.getByText(/a issue e os pull requests vinculados a ela/i),
    ).toBeInTheDocument();
    expect(screen.queryByText(/somente desta referência/i)).toBeNull();
    expect(asked[0]?.searchParams.get("ref")).toBe(REF);
  });

  it("marks a run that came by a linked pull request, and names that pull request", async () => {
    serve({
      "": {
        ref: REF,
        runs: [
          run("run-dev", "developer", "2026-10-01T09:00:00Z"),
          {
            ...run("run-review", "reviewer", "2026-10-01T10:00:00Z"),
            link: "alias" as const,
            ref: PR,
          },
        ],
        roles: [],
      },
    });
    open();

    const review = await screen.findByRole("row", { name: /run-review/ });
    expect(within(review).getByText(/pull request/i)).toBeInTheDocument();
    expect(within(review).getByText(PR)).toBeInTheDocument();
    const dev = screen.getByRole("row", { name: /run-dev/ });
    expect(within(dev).getByText(/assumiu/i)).toBeInTheDocument();
    expect(within(dev).queryByText(PR)).toBeNull();
  });

  it("marks runs that inherited the card, and names the card an inherited run claimed", async () => {
    const OTHER = "github:fuseone-io/fuseone-agents#300";
    serve({
      "": {
        ref: REF,
        runs: [
          {
            ...run("run-qa", "qa", "2026-10-01T09:00:00Z"),
            link: "event" as const,
            claimedRef: OTHER,
          },
          {
            ...run("run-hook", "triage", "2026-10-01T10:00:00Z"),
            link: "github_delivery" as const,
          },
        ],
        roles: [],
      },
    });
    open();

    const qa = await screen.findByRole("row", { name: /run-qa/ });
    expect(within(qa).getByText("Evento")).toBeInTheDocument();
    expect(
      within(qa).getByRole("link", { name: `Também assumiu ${OTHER}` }),
    ).toHaveAttribute("href", workItemPath(OTHER));
    const hook = screen.getByRole("row", { name: /run-hook/ });
    expect(within(hook).getByText("Entrega do GitHub")).toBeInTheDocument();
    expect(within(hook).queryByText(REF)).toBeNull();
  });

  it.each([
    ["pt-BR", "Entrou na trilha", "Primeira atribuição", /assum/i],
    ["en-US", "Joined the trail", "First attribution", /claim/i],
  ])(
    "says an inherited run joined the trail, never that it claimed the card (%s)",
    async (locale, joined, first, claimWord) => {
      await i18next.changeLanguage(locale);
      try {
        serve({ "": inheritedOnly });
        open();
        const hook = await screen.findByRole("row", { name: /run-hook/ });
        expect(
          screen.getByRole("columnheader", { name: joined }),
        ).toBeInTheDocument();
        expect(
          screen.getByRole("columnheader", { name: first }),
        ).toBeInTheDocument();
        expect(within(hook).queryByText(claimWord)).toBeNull();
        expect(
          screen.queryByRole("columnheader", { name: claimWord }),
        ).toBeNull();
      } finally {
        await i18next.changeLanguage("pt-BR");
      }
    },
  );

  it("opened by a linked pull request, shows the card's trail and says where it came from", async () => {
    serve({ "": { ...firstPage, resolvedFrom: PR } });
    open(workItemPath(PR));

    expect(await screen.findByText(REF)).toBeInTheDocument();
    expect(
      screen.getByText(/um pull request vinculado a este item/i),
    ).toHaveTextContent(PR);
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

  it("asks for no trail without a reference: the weekly panel is shown instead", async () => {
    const asked = serve({});
    open("/work-items");

    expect(
      screen.getByRole("heading", { name: "Por papel, por semana" }),
    ).toBeInTheDocument();
    await vi.waitFor(() => expect(asked.length).toBeGreaterThan(0));
    expect(asked.some((u) => u.pathname.endsWith("/trail"))).toBe(false);
  });
});

describe("the path to a work item", () => {
  it("encodes the reference so the issue number survives the URL", () => {
    expect(workItemPath(REF)).toBe(
      "/work-items?ref=github%3Afuseone-io%2Ffuseone-agents%23204",
    );
  });
});
