import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MCPServerPage } from "@/features/integrations/mcp/mcp-server-page";
import { setLocale } from "@/i18n";
import { json, revisedCollection, stubNetwork } from "@/test/revised-routes";
import { scopeListsRoute } from "@/test/scope-lists-routes";

/*
The move is bound to the revision the page opened on (NT-019). Anything that
reads the server again, a refused move included, must not hand the next try a
revision nobody looked at: only the page's reload does.
*/

beforeEach(() => {
  setLocale("en-US");
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
});
afterEach(() => vi.unstubAllGlobals());

function world() {
  const servers = revisedCollection("/admin/integrations/mcp-servers", [
    { name: "wiki", transport: "http", url: "https://wiki.example.com/mcp", enabled: true },
  ]);
  const requests = stubNetwork([servers], (method, path) => {
    const listed = scopeListsRoute(method, path);
    if (listed) return listed;
    if (path === "/admin/integrations") return json({ mcpServers: servers.state.items, providers: [] });
    if (path.endsWith("/rediscovery")) return json({ server: "wiki", generation: 0, receipts: [] });
    if (path.endsWith("/move")) {
      return json({ type: "fuseone:egress-not-allowed", title: "refused", status: 400, detail: "https://wiki.example.com" }, 400);
    }
    return json({ items: [] });
  });
  return { servers, moves: () => requests.filter((r) => r.path.endsWith("/move")) };
}

describe("moving a server from its page", () => {
  it("sends the revision the page opened on, after a refusal read the server again", async () => {
    const { servers, moves } = world();
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <MemoryRouter initialEntries={["/integrations/mcp/wiki"]}>
          <Routes><Route path="/integrations/mcp/:name" element={<MCPServerPage />} /></Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Move to…" }));
    const opened = servers.etag("wiki");
    servers.elsewhere("wiki", { url: "https://wiki.example.com/other" });
    await user.click(screen.getByRole("combobox", { name: "Company" }));
    await user.click(screen.getByRole("option", { name: "Acme" }));
    await user.click(screen.getByRole("button", { name: "Move" }));
    await waitFor(() => expect(moves()).toHaveLength(1));
    await user.click(screen.getByRole("button", { name: "Move" }));

    await waitFor(() => expect(moves()).toHaveLength(2));
    expect(moves().map((m) => m.ifMatch)).toEqual([opened, opened]);
  });
});
