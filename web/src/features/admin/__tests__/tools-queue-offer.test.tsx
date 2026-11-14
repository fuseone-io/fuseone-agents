import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ToolsPanel } from "@/features/admin/tools-panel";
import type { Tool } from "@/features/admin/api";
import { setLocale } from "@/i18n";
import { json, type Recorded } from "@/test/revised-routes";

/*
The administration's queue, for an area's curator (NT-019).

They see it now that tool:classify is theirs where they hold it. The queue is
asked for each scope they classify at, and a tool is offered for a ruling
only where they may record one: as the owner, on their own scope's tools,
never on the installation's.
*/

const AREA_CURATOR = {
  id: "usr_ana", display: "Ana", kind: "user", grants: [],
  can: ["tool:read", "tool:classify"], platform: [],
  classify: [{ company: "acme", area: "cx" }],
};

const waiting = (toolId: string, company: string, area = ""): Tool => ({
  toolId, server: toolId.split(".")[0] ?? "", effect: "read", stale: true,
  untrusted: false, digest: "sha-1", scope: { company, area },
  rulings: [{ level: "owner", scope: { company, area }, effect: "read", untrusted: false, inForce: true }],
});
const TOOLS = [waiting("crm.search", "acme", "cx"), waiting("shared.search", "*")];

/*
As the server answers: a target holds the tools whose scope contains it, which
for `acme` is the company's and the installation's and never `acme/cx`'s;
`classifiable` holds what the caller governs (the stub's two tools stand for
it, the installation's included, so the per-row guard is still exercised).
*/
function answered(query: URLSearchParams): Tool[] {
  if (query.get("classifiable") === "true") return TOOLS;
  const [company, area = ""] = (query.get("scope") ?? "*").split("/");
  return TOOLS.filter(({ scope }) =>
    scope?.company === "*" || (scope?.company === company && (scope?.area === "" || scope?.area === area)));
}

function world(me: object) {
  const asked: Recorded[] = [];
  const urls: URL[] = [];
  vi.stubGlobal("fetch", async (input: Request | string) => {
    const request = typeof input === "string" ? new Request(new URL(input, "http://localhost")) : input;
    const url = new URL(request.url);
    const path = url.pathname.replace(/^\/api\/v1/, "");
    const text = request.method === "GET" ? "" : await request.text();
    asked.push({ method: request.method, path, ifMatch: null, body: text ? JSON.parse(text) : undefined });
    urls.push(url);
    if (path === "/me") return json(me);
    if (path === "/admin/tools") return json({ items: answered(url.searchParams) });
    if (path.endsWith("/classification")) return json({});
    return json({}, 404);
  });
  return { asked, urls };
}

function show() {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter>
        <ToolsPanel />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const row = (toolId: string) => screen.getByText(toolId).closest("tr") as HTMLElement;

beforeEach(() => setLocale("pt-BR"));
afterEach(() => vi.unstubAllGlobals());

describe("an area's curator in the queue", () => {
  it("asks once for what they govern, not for a target", async () => {
    const { urls } = world(AREA_CURATOR);
    show();

    await screen.findByText("crm.search");
    const asked = urls.filter((u) => u.pathname.endsWith("/admin/tools"));
    expect(asked.map((u) => u.search)).toEqual(["?classifiable=true"]);
  });

  it("is offered their own tool, not the installation's", async () => {
    world(AREA_CURATOR);
    show();

    await screen.findByText("crm.search");
    expect(within(row("crm.search")).getByRole("button", { name: "Classificar" })).toBeInTheDocument();
    expect(within(row("shared.search")).queryByRole("button", { name: "Classificar" })).toBeNull();
  });

  it("records the owner's ruling on their own tool", async () => {
    const user = userEvent.setup();
    const { asked } = world(AREA_CURATOR);
    show();

    await screen.findByText("crm.search");
    await user.click(within(row("crm.search")).getByRole("button", { name: "Classificar" }));
    await user.click(screen.getByRole("button", { name: "Registrar classificação" }));

    await waitFor(() => expect(asked.some((r) => r.method === "PUT")).toBe(true));
    expect(asked.find((r) => r.method === "PUT")?.body).toMatchObject({ level: "owner" });
  });
});

/*
A company's grant classifies its areas' tools. A target answer for `acme`
holds only what contains acme, so the queue asks what the curator governs.
*/
describe("a company's curator in the queue", () => {
  it("is offered a tool of one of the company's areas", async () => {
    world({ ...AREA_CURATOR, classify: [{ company: "acme", area: "" }] });
    show();

    await screen.findByText("crm.search");
    expect(within(row("crm.search")).getByRole("button", { name: "Classificar" })).toBeInTheDocument();
  });
});
