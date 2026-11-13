import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ServerEditor } from "@/features/integrations/server-editor";
import { MCPServerPage } from "@/features/integrations/mcp/mcp-server-page";
import type { MCPServer } from "@/features/integrations/api";
import { setLocale } from "@/i18n";
import { json, revisedCollection, stubNetwork } from "@/test/revised-routes";

// NT-019: a server's scope does not move. Every write of one stored at an area
// sends that scope back; written without it, the name is the installation's,
// another scope already holds it, and the write is refused.

const crm: MCPServer = {
  name: "crm", transport: "http", url: "https://crm.example.com/mcp", enabled: true,
  scopeKind: "area", company: "acme", area: "cx", editable: true,
};
const stored = { scopeKind: "area", company: "acme", area: "cx" };

function world() {
  const servers = revisedCollection<{ name: string }>("/admin/integrations/mcp-servers", [crm]);
  const requests = stubNetwork([servers], (_m, path) => {
    if (path === "/admin/integrations") return json({ mcpServers: servers.state.items, providers: [] });
    if (path === "/admin/tools") return json({ items: [{ toolId: "crm.search", server: "crm", effect: "read" }] });
    if (path.endsWith("/rediscovery")) return json({ server: "crm", generation: 0, receipts: [] });
    return undefined;
  });
  return () => requests.filter((r) => r.method === "PUT");
}

function show(node: ReactNode, path = "/") {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/integrations/mcp/:name" element={<MCPServerPage />} />
          <Route path="*" element={node} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  setLocale("pt-BR");
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
});
afterEach(() => vi.unstubAllGlobals());

describe("an area's server, written again", () => {
  it("is saved from its sheet at the scope it was read at, which the sheet does not offer to change", async () => {
    const puts = world();
    show(<ServerEditor server={crm} onClose={vi.fn()} />);
    await userEvent.setup().click(await screen.findByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(puts()).toHaveLength(1));
    expect(puts()[0]?.body).toMatchObject(stored);
    expect(screen.getByRole("combobox", { name: "Escopo do servidor" })).toBeDisabled();
  });

  it("keeps its scope when its surface is saved", async () => {
    const user = userEvent.setup();
    const puts = world();
    show(null, "/integrations/mcp/crm");
    await user.click(await screen.findByLabelText("crm.search"));
    await user.click(screen.getByRole("button", { name: "Salvar o que trouxemos" }));

    await waitFor(() => expect(puts()).toHaveLength(1));
    expect(puts()[0]?.body).toMatchObject(stored);
  });

  it("keeps its scope when its credential is saved", async () => {
    const user = userEvent.setup();
    const puts = world();
    show(null, "/integrations/mcp/crm");
    await user.type(await screen.findByLabelText("Token bearer"), "typed-secret");
    await user.click(screen.getByRole("button", { name: "Salvar a credencial" }));

    await waitFor(() => expect(puts()).toHaveLength(1));
    expect(puts()[0]?.body).toMatchObject({ ...stored, token: "typed-secret" });
  });
});
