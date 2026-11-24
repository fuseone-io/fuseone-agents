import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ServerEditor } from "@/features/integrations/server-editor";
import { MCPServerPage } from "@/features/integrations/mcp/mcp-server-page";
import type { MCPServer } from "@/features/integrations/api";
import { setLocale } from "@/i18n";
import { json, revisedCollection, stubNetwork } from "@/test/revised-routes";

/*
The custom header is for a server no recipe describes. Where a recipe says what
the server takes, offering a free header would let anyone replace the
credential the recipe prescribes; while the recipe is unread, nobody knows.
*/
const recipe = {
  server: "github", title: "GitHub", category: "dev", publisher: "GitHub", docsFrom: "publisher",
  provenance: "documentation", status: "published", configRequirements: ["credential"],
  requiresPersonalCredential: false, authModes: [{ type: "bearer", principal: "service" }],
  transport: "http", url: "https://mcp.example.com/a",
};
const server = (name: string): MCPServer => ({ name, transport: "http", url: "https://mcp.example.com/a", enabled: true });

function world(name: string) {
  const servers = revisedCollection("/admin/integrations/mcp-servers", [server(name)]);
  stubNetwork([servers], (_method, path) => {
    if (path === "/admin/integrations/recipes") return json({ items: [recipe] });
    if (path === "/admin/integrations") return json({ mcpServers: servers.state.items, providers: [] });
    if (path.endsWith("/rediscovery")) return json({ server: name, generation: 0, receipts: [] });
    if (path === "/admin/tools") return json({ items: [] });
    return undefined;
  });
}

function show(node: ReactNode, path = "/") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/integrations/mcp/:name" element={<MCPServerPage />} />
          <Route path="*" element={node} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("the custom header follows the server's recipe", () => {
  beforeEach(() => {
    setLocale("en-US");
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.scrollIntoView ??= () => {};
  });
  afterEach(() => vi.unstubAllGlobals());

  it.each([
    ["the server's page", (name: string) => show(null, `/integrations/mcp/${name}`)],
    ["the connected editor", (name: string) => show(<ServerEditor server={server(name)} onClose={() => {}} />)],
  ])("on %s, is offered for a server with no recipe", async (_where, open) => {
    world("crm");
    open("crm");
    expect(await screen.findByLabelText("Header name")).toBeInTheDocument();
  });

  it.each([
    ["the server's page", (name: string) => show(null, `/integrations/mcp/${name}`)],
    ["the connected editor", (name: string) => show(<ServerEditor server={server(name)} onClose={() => {}} />)],
  ])("on %s, is not offered where a recipe says what the server takes", async (_where, open) => {
    world("github");
    open("github");
    expect((await screen.findAllByLabelText(/bearer token/i)).length).toBeGreaterThan(0);
    await new Promise((settle) => setTimeout(settle, 200));
    expect(screen.queryByLabelText("Header name")).not.toBeInTheDocument();
  });
});
