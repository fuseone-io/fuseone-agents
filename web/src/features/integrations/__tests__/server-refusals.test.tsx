import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { Toaster } from "@/components/ui/sonner";
import { ServerEditor } from "@/features/integrations/server-editor";
import { MCPServerPage } from "@/features/integrations/mcp/mcp-server-page";
import { ScopeChoiceContext } from "@/features/integrations/scope/scope-choice";
import { setLocale } from "@/i18n";
import { json, stubNetwork } from "@/test/revised-routes";

// NT-019: what the server refuses about an area's new server is said beside
// the field it is about, in the reader's language.

const egress = {
  type: "fuseone:egress-not-allowed", title: "Egress not allowed",
  detail: "https://elsewhere.example.org:443",
};

function page() {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={["/integrations/mcp/crm"]}>
        <Routes>
          <Route path="/integrations/mcp/:name" element={<MCPServerPage />} />
        </Routes>
      </MemoryRouter>
      <Toaster />
    </QueryClientProvider>,
  );
}

function refusedWith(status: number, problem: Record<string, unknown>) {
  stubNetwork([], (method) => (method === "PUT" ? json({ status, ...problem }, status) : undefined));
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ScopeChoiceContext.Provider value={{ anyScope: false, configure: [{ company: "acme", area: "cx" }] }}>
        <ServerEditor server={null} onClose={vi.fn()} />
      </ScopeChoiceContext.Provider>
      <Toaster />
    </QueryClientProvider>,
  );
}

async function save() {
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("Nome"), "crm");
  await user.type(screen.getByLabelText("Endereço"), "https://crm.example.com/mcp");
  await user.click(screen.getByRole("button", { name: "Salvar" }));
}

beforeEach(() => {
  setLocale("pt-BR");
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
});
afterEach(() => vi.unstubAllGlobals());

describe("a new server the server refuses", () => {
  it("says the name is another scope's", async () => {
    refusedWith(409, {
      type: "fuseone:integration-name-taken", title: "Name taken",
      detail: "another scope already holds this name; choose another",
    });
    await save();

    expect(await screen.findByText("Outro escopo já usa este nome. Escolha outro.")).toBeInTheDocument();
  });

  it("says the address is outside the platform's egress policy, naming the origin", async () => {
    refusedWith(400, egress);
    await save();

    expect(await screen.findByText(/fora da política de egresso da plataforma/)).toHaveTextContent(
      "https://elsewhere.example.org:443",
    );
  });

  // The type decides, never the words: an invalid input that happens to say
  // "egress" is any other refusal.
  it("does not take an invalid input mentioning egress for the egress refusal", async () => {
    refusedWith(400, {
      type: "fuseone:invalid-input", title: "Invalid request",
      detail: "outside the platform's egress policy: https://elsewhere.example.org:443",
    });
    await save();

    expect(await screen.findByText(/O pedido não pôde ser aceito/)).toBeInTheDocument();
    expect(screen.queryByText(/fora da política de egresso da plataforma/)).not.toBeInTheDocument();
  });
});

// A policy narrowed after the server was created refuses its next write, the
// surface's included, and that write says so in the same words.
describe("an existing server's surface the server refuses", () => {
  it("says the server is outside the platform's egress policy, naming the origin", async () => {
    const crm = { name: "crm", transport: "http", url: "https://crm.example.com/mcp", enabled: true };
    stubNetwork([], (method, path) => {
      if (method === "PUT") return json({ status: 400, ...egress }, 400);
      if (path === "/admin/integrations/mcp-servers/crm") return json(crm, 200, { ETag: '"v1"' });
      if (path === "/admin/integrations") return json({ mcpServers: [crm], providers: [] });
      if (path === "/admin/tools") return json({ items: [{ toolId: "crm.search", server: "crm", effect: "read" }] });
      return path.endsWith("/rediscovery") ? json({ server: "crm", generation: 0, receipts: [] }) : undefined;
    });
    page();
    const user = userEvent.setup();
    await user.click(await screen.findByLabelText("crm.search"));
    await user.click(screen.getByRole("button", { name: "Salvar o que trouxemos" }));

    expect(await screen.findByText(/fora da política de egresso da plataforma/)).toHaveTextContent(
      "https://elsewhere.example.org:443",
    );
  });
});
