import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MCPServerPage } from "@/features/integrations/mcp/mcp-server-page";
import type { MCPServer } from "@/features/integrations/api";
import type { Tool } from "@/features/admin/api";
import { setLocale } from "@/i18n";
import { json, type Recorded } from "@/test/revised-routes";

/*
Classifying from a server's page, under the platform's ceiling (NT-019).

An area's curator rules on their own servers' tools as the owner; the
installation's servers are not theirs to rule on and are not offered. The
platform's curator keeps ruling as the platform by default, and is the only
one offered withdrawing the ceiling.
*/

const AREA_CURATOR = {
  can: ["tool:read", "provider:write", "tool:classify"], platform: [],
  classify: [{ company: "acme", area: "cx" }],
};
const PLATFORM_CURATOR = {
  can: ["tool:read", "tool:classify", "provider:write"],
  platform: ["tool:read", "tool:classify", "provider:write"],
};

const crm: MCPServer = {
  name: "crm", transport: "http", url: "https://crm.example.com/mcp", enabled: true,
  scopeKind: "area", company: "acme", area: "cx", editable: true,
};
const shared: MCPServer = { ...crm, name: "shared", scopeKind: "installation", company: "*", area: "", editable: false };

const ruled = (over: Partial<Tool> = {}): Tool => ({
  toolId: "crm.search", server: "crm", effect: "read", untrusted: false, digest: "sha-1",
  scope: { company: "acme", area: "cx" },
  // Ruled read at both levels, as the listing says of a ruled tool.
  rulings: [
    { level: "platform", scope: { company: "*", area: "" }, effect: "read", untrusted: false, inForce: true },
    { level: "owner", scope: { company: "acme", area: "cx" }, effect: "read", untrusted: false, inForce: false, overridden: true },
  ],
  ...over,
});

function world(me: object, tools: Tool[]) {
  const asked: Recorded[] = [];
  const urls: URL[] = [];
  vi.stubGlobal("fetch", async (input: Request | string) => {
    // The session is asked for by a bare path, everything else by a Request.
    const request = typeof input === "string" ? new Request(new URL(input, "http://localhost")) : input;
    const url = new URL(request.url);
    const path = url.pathname.replace(/^\/api\/v1/, "");
    const text = request.method === "GET" ? "" : await request.text();
    asked.push({ method: request.method, path, ifMatch: null, body: text ? JSON.parse(text) : undefined });
    urls.push(url);
    if (path === "/me") return json({ id: "usr_ana", display: "Ana", kind: "user", grants: [], ...me });
    if (path === "/admin/integrations") return json({ mcpServers: [crm, shared], providers: [] });
    const server = [crm, shared].find((s) => path === `/admin/integrations/mcp-servers/${s.name}`);
    if (server) return json(server, 200, { ETag: '"v1"' });
    if (path === "/admin/tools") return json({ items: tools });
    if (path.endsWith("/rediscovery")) return json({ server: "crm", generation: 0, receipts: [] });
    if (path.endsWith("/classification")) return json({}, request.method === "PUT" ? 200 : 204);
    return json({}, 404);
  });
  return { asked, urls };
}

function show(name = "crm") {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={[`/integrations/mcp/${name}`]}>
        <Routes>
          <Route path="/integrations/mcp/:name" element={<MCPServerPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const classified = (asked: Recorded[]) =>
  asked.filter((r) => r.method === "PUT" && r.path.endsWith("/classification"));

async function recordRuling() {
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Mudar a decisão" }));
  await user.click(screen.getByRole("button", { name: "Registrar classificação" }));
}

beforeEach(() => {
  setLocale("pt-BR");
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
});
afterEach(() => vi.unstubAllGlobals());

describe("an area's curator on their own server", () => {
  it("asks for the tools at the server's scope", async () => {
    const { urls } = world(AREA_CURATOR, [ruled()]);
    show();

    await screen.findByRole("button", { name: "Mudar a decisão" });
    const tools = urls.filter((u) => u.pathname.endsWith("/admin/tools"));
    expect(tools.map((u) => u.searchParams.get("scope"))).toEqual(["acme/cx"]);
  });

  it("records the ruling as the owner", async () => {
    const { asked } = world(AREA_CURATOR, [ruled()]);
    show();

    await recordRuling();

    await waitFor(() => expect(classified(asked)).toHaveLength(1));
    expect(classified(asked)[0]?.body).toMatchObject({ level: "owner", effect: "read" });
  });

  it("is not offered a ruling on the installation's server", async () => {
    world(AREA_CURATOR, [ruled({ toolId: "shared.search", server: "shared", scope: { company: "*", area: "" } })]);
    show("shared");

    expect(await screen.findByLabelText("shared.search")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Mudar a decisão" })).not.toBeInTheDocument();
  });
});

describe("the platform's curator", () => {
  it("records the platform's ruling by default", async () => {
    const { asked } = world(PLATFORM_CURATOR, [ruled()]);
    show();

    await recordRuling();

    await waitFor(() => expect(classified(asked)).toHaveLength(1));
    expect(classified(asked)[0]?.body).not.toHaveProperty("level");
  });

  it("may record the owner's ruling on an area's server instead", async () => {
    const user = userEvent.setup();
    const { asked } = world(PLATFORM_CURATOR, [ruled()]);
    show();
    await user.click(await screen.findByRole("button", { name: "Mudar a decisão" }));

    await user.click(screen.getByRole("radio", { name: /Decisão do dono/ }));
    await user.click(screen.getByRole("button", { name: "Registrar classificação" }));

    await waitFor(() => expect(classified(asked)).toHaveLength(1));
    expect(classified(asked)[0]?.body).toMatchObject({ level: "owner" });
  });

  it("withdraws the platform's ruling, and the owner's is shown overridden until then", async () => {
    const user = userEvent.setup();
    const rulings: Tool["rulings"] = [
      { level: "platform", scope: { company: "*", area: "" }, effect: "read", untrusted: false, inForce: true },
      { level: "owner", scope: { company: "acme", area: "cx" }, effect: "write", untrusted: false, inForce: false, overridden: true },
    ];
    const { asked } = world(PLATFORM_CURATOR, [ruled({ rulings })]);
    show();
    await user.click(await screen.findByRole("button", { name: "Mudar a decisão" }));

    const shown = screen.getByRole("list", { name: "Decisões sobre a ferramenta" });
    expect(within(shown).getByText("Sobreposta pela plataforma")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Retirar o teto da plataforma" }));
    await user.click(await screen.findByRole("button", { name: "Retirar" }));

    await waitFor(() => expect(asked.some((r) => r.method === "DELETE")).toBe(true));
  });
});

describe("an area's curator and the ceiling", () => {
  it("sees the rulings but is not offered withdrawing the platform's", async () => {
    const rulings: Tool["rulings"] = [
      { level: "platform", scope: { company: "*", area: "" }, effect: "read", untrusted: false, inForce: true },
    ];
    world(AREA_CURATOR, [ruled({ rulings })]);
    show();
    await userEvent.setup().click(await screen.findByRole("button", { name: "Mudar a decisão" }));

    expect(screen.getByRole("list", { name: "Decisões sobre a ferramenta" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Retirar o teto da plataforma" })).not.toBeInTheDocument();
  });
});

/*
Editing one level starts from that level's ruling. The tool's own fields are
the ruling in force, the platform's under a ceiling: started from them, an
owner saving their ruling would write the platform's word over their own and
lose its compensation and dedupe.
*/
const UNDER_CEILING: Tool["rulings"] = [
  { level: "platform", scope: { company: "*", area: "" }, effect: "read", untrusted: false, inForce: true },
  {
    level: "owner", scope: { company: "acme", area: "cx" }, effect: "write", untrusted: true,
    inForce: false, overridden: true, compensatedBy: "crm.undo",
    dedupe: { windowSeconds: 3600, argPaths: ["ticket.id"] },
  },
];
const OWNERS_WORD = {
  level: "owner", effect: "write", untrusted: true, compensatedBy: "crm.undo",
  dedupe: { windowSeconds: 3600, argPaths: ["ticket.id"] },
};

describe("editing one level's ruling", () => {
  it("starts the owner from the owner's ruling, not the ceiling in force", async () => {
    const { asked } = world(AREA_CURATOR, [ruled({ rulings: UNDER_CEILING })]);
    show();

    await recordRuling();

    await waitFor(() => expect(classified(asked)).toHaveLength(1));
    expect(classified(asked)[0]?.body).toMatchObject(OWNERS_WORD);
  });

  it("starts again from the other level's ruling when the platform switches", async () => {
    const user = userEvent.setup();
    const { asked } = world(PLATFORM_CURATOR, [ruled({ rulings: UNDER_CEILING })]);
    show();
    await user.click(await screen.findByRole("button", { name: "Mudar a decisão" }));

    await user.click(screen.getByRole("radio", { name: /Decisão do dono/ }));
    await user.click(screen.getByRole("button", { name: "Registrar classificação" }));

    await waitFor(() => expect(classified(asked)).toHaveLength(1));
    expect(classified(asked)[0]?.body).toMatchObject(OWNERS_WORD);
  });

  it("starts a level nobody ruled at with no effect at all", async () => {
    const user = userEvent.setup();
    world(PLATFORM_CURATOR, [ruled({ rulings: UNDER_CEILING.slice(0, 1) })]);
    show();
    await user.click(await screen.findByRole("button", { name: "Mudar a decisão" }));

    await user.click(screen.getByRole("radio", { name: /Decisão do dono/ }));

    expect(screen.getByRole("button", { name: "Registrar classificação" })).toBeDisabled();
  });
});
