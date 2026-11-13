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
});
const TOOLS = [waiting("crm.search", "acme", "cx"), waiting("shared.search", "*")];

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
    if (path === "/admin/tools") return json({ items: TOOLS });
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
  it("asks for the tools of each scope they classify at", async () => {
    const { urls } = world(AREA_CURATOR);
    show();

    await screen.findByText("crm.search");
    const scopes = urls.filter((u) => u.pathname.endsWith("/admin/tools"));
    expect(scopes.map((u) => u.searchParams.get("scope"))).toEqual(["acme/cx"]);
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
