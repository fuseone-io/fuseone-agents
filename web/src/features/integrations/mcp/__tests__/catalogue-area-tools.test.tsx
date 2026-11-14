import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CataloguePage } from "@/features/integrations/mcp/catalogue-page";

// Without a target the tool catalogue answers an area's reader the
// installation's tools alone (NT-019). The catalogue asks for the scope of
// each server it shows, so an area's own server lists its tools.

const crm = {
  name: "crm", transport: "http", url: "https://crm.example.com/mcp", enabled: true,
  scopeKind: "area", company: "acme", area: "cx", editable: true,
};
const closeTicket = { toolId: "crm.close_ticket", server: "crm", effect: "write", untrusted: false };
// Stored at the company, which the area inherits; the area's administrator
// may not ask for the company as a target.
const wiki = {
  name: "wiki", transport: "http", url: "https://wiki.example.com/mcp", enabled: true,
  scopeKind: "company", company: "acme", editable: false,
};
const readPage = { toolId: "wiki.read_page", server: "wiki", effect: "read", untrusted: false };

function signedInAsAreaAdmin() {
  const me = {
    id: "usr_bia", display: "Bia", kind: "human", grants: [], platform: [],
    can: ["tool:read", "provider:write"], configure: [{ company: "acme", area: "cx" }],
  };
  vi.stubGlobal("fetch", vi.fn(async (input: Request | string) => {
    const url = new URL(input instanceof Request ? input.url : String(input), "http://console");
    const body = url.pathname.endsWith("/me")
      ? me
      : url.pathname.endsWith("/admin/tools")
        ? toolsFor(url.searchParams.get("scope"))
        : { items: [], mcpServers: [crm, wiki], providers: [] };
    if (body === null) return new Response(JSON.stringify({ title: "not found", status: 404 }), { status: 404 });
    return new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } });
  }));
}

// As the server answers: the area's own target holds what it inherits, and a
// target above the caller's grants is not found.
function toolsFor(scope: string | null) {
  if (scope === "acme/cx") return { items: [closeTicket, readPage] };
  if (scope === null) return { items: [] };
  return null;
}

afterEach(() => vi.unstubAllGlobals());

describe("the catalogue for an area's administrator", () => {
  it("lists the tools of a server of their area", async () => {
    signedInAsAreaAdmin();
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <MemoryRouter><CataloguePage /></MemoryRouter>
      </QueryClientProvider>,
    );
    await userEvent.setup().click(await screen.findByRole("button", { name: /crm/ }, { timeout: 5000 }));

    expect(await screen.findByText("crm.close_ticket")).toBeInTheDocument();
    expect(screen.queryByText(/Nenhuma ferramenta publicada/)).not.toBeInTheDocument();
  });

  it("lists the tools of a server their area inherits from its company, never asking for the company", async () => {
    signedInAsAreaAdmin();
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <MemoryRouter><CataloguePage /></MemoryRouter>
      </QueryClientProvider>,
    );
    await userEvent.setup().click(await screen.findByRole("button", { name: /wiki/ }, { timeout: 5000 }));

    expect(await screen.findByText("wiki.read_page")).toBeInTheDocument();
    const scopes = vi.mocked(fetch).mock.calls
      .map(([input]) => new URL(input instanceof Request ? input.url : String(input), "http://console"))
      .filter((url) => url.pathname.endsWith("/admin/tools"))
      .map((url) => url.searchParams.get("scope"));
    expect(scopes).not.toContain("acme");
  });
});
