import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Toaster } from "@/components/ui/sonner";
import type { MCPServer } from "@/features/integrations/api";
import { ServerEditor } from "@/features/integrations/server-editor";
import { ScopeChoiceContext, type ScopeChoice } from "@/features/integrations/scope/scope-choice";
import { useActiveScope } from "@/features/scope/active-scope";
import { setLocale } from "@/i18n";
import { json, revisedCollection, stubNetwork } from "@/test/revised-routes";
import { scopeListsRoute } from "@/test/scope-lists-routes";

// NT-019: a new server is placed at a scope, and outside the installation it
// is remote, because a local one runs on the platform's worker.

const areaAdmin: ScopeChoice = { anyScope: false, configure: [{ company: "acme", area: "cx" }] };

function connect(choice?: ScopeChoice, server: MCPServer | null = null) {
  const servers = revisedCollection<{ name: string }>("/admin/integrations/mcp-servers", server ? [server] : []);
  const requests = stubNetwork([servers], (method, path) =>
    path === "/admin/integrations"
      ? json({ mcpServers: [], providers: [] })
      : scopeListsRoute(method, path),
  );
  const editor = <ServerEditor server={server} onClose={vi.fn()} />;
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      {choice ? <ScopeChoiceContext.Provider value={choice}>{editor}</ScopeChoiceContext.Provider> : editor}
      <Toaster />
    </QueryClientProvider>,
  );
  return () => requests.filter((r) => r.method === "PUT");
}

async function fill(name: string, url: string) {
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("Nome"), name);
  await user.type(screen.getByLabelText("Endereço"), url);
  await user.click(screen.getByRole("button", { name: "Salvar" }));
}

beforeEach(() => {
  setLocale("pt-BR");
  useActiveScope.setState({ company: "", area: "" });
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
});
afterEach(() => vi.unstubAllGlobals());

describe("a new server's scope", () => {
  it("is one the area's administrator configures, and is sent with the write", async () => {
    const puts = connect(areaAdmin);

    expect(screen.getByRole("combobox", { name: "Onde o servidor vale" })).toHaveTextContent("acme/cx");
    await fill("crm", "https://crm.example.com/mcp");

    await waitFor(() => expect(puts()).toHaveLength(1));
    expect(puts()[0]?.body).toMatchObject({
      scopeKind: "area", company: "acme", area: "cx", transport: "http",
    });
  });

  it("offers only a remote address outside the installation, and says why", async () => {
    connect(areaAdmin);
    await userEvent.setup().click(screen.getByRole("combobox", { name: "Como é alcançado" }));

    expect(await screen.findByRole("option", { name: "Endereço remoto (HTTP)" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Processo local (stdio)" })).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByText(/Fora da instalação, o servidor é sempre remoto/)).toBeInTheDocument();
    expect(screen.queryByText("Este servidor executa código dentro do worker")).not.toBeInTheDocument();
  });

  it("is free for the platform's administration, the installation by default", async () => {
    const puts = connect();
    await userEvent.setup().click(screen.getByRole("combobox", { name: "Como é alcançado" }));
    await userEvent.setup().click(await screen.findByRole("option", { name: "Endereço remoto (HTTP)" }));
    await fill("crm", "https://crm.example.com/mcp");

    await waitFor(() => expect(puts()).toHaveLength(1));
    expect(screen.getByRole("combobox", { name: "Escopo do servidor" })).toHaveTextContent("Instalação");
    expect(puts()[0]?.body).toMatchObject({ scopeKind: "installation" });
  });


  // The scope the console is reading is where a new server starts, when a
  // grant reaches it: not the first grant listed.
  it("starts on the active area that a company grant contains", async () => {
    useActiveScope.setState({ company: "acme", area: "cx" });
    const puts = connect({ anyScope: false, configure: [{ company: "acme", area: "" }] });

    expect(screen.getByRole("combobox", { name: "Onde o servidor vale" })).toHaveTextContent("acme/cx");
    await fill("crm", "https://crm.example.com/mcp");
    await waitFor(() => expect(puts()).toHaveLength(1));
    expect(puts()[0]?.body).toMatchObject({ scopeKind: "area", company: "acme", area: "cx" });
  });

  it("starts on the active area rather than the first one granted", () => {
    useActiveScope.setState({ company: "acme", area: "cx" });
    connect({ anyScope: false, configure: [{ company: "acme", area: "finance" }, { company: "acme", area: "cx" }] });

    expect(screen.getByRole("combobox", { name: "Onde o servidor vale" })).toHaveTextContent("acme/cx");
  });
});

async function choose(name: string, option: string) {
  const user = userEvent.setup();
  await user.click(await screen.findByRole("combobox", { name }));
  await user.click(await screen.findByRole("option", { name: option }));
}

async function optionsOf(name: string) {
  await waitFor(() => expect(screen.getByRole("combobox", { name })).toBeEnabled());
  await userEvent.setup().click(screen.getByRole("combobox", { name }));
  return screen.getAllByRole("option").map((option) => option.textContent);
}

// The platform's administration places a server at any registered scope,
// chosen from the lists rather than typed.
describe("the platform's free scope", () => {
  it("offers the registered companies, never a withdrawn one", async () => {
    connect();
    await choose("Escopo do servidor", "Área");

    expect(await optionsOf("Empresa")).toEqual(["Acme", "Example"]);
  });

  it("offers only the chosen company's areas, and an area is required", async () => {
    connect();
    await choose("Escopo do servidor", "Área");
    await choose("Empresa", "Acme");

    expect(await optionsOf("Área")).toEqual(["Financeiro", "Operações"]);
  });

  it("sends the chosen company and area", async () => {
    const puts = connect();
    await choose("Escopo do servidor", "Área");
    await choose("Empresa", "Acme");
    await choose("Área", "Operações");
    await userEvent.setup().click(screen.getByRole("combobox", { name: "Como é alcançado" }));
    await userEvent.setup().click(await screen.findByRole("option", { name: "Endereço remoto (HTTP)" }));
    await fill("crm", "https://crm.example.com/mcp");

    await waitFor(() => expect(puts()).toHaveLength(1));
    expect(puts()[0]?.body).toMatchObject({ scopeKind: "area", company: "acme", area: "ops" });
  });

  it("refuses an area server with no area chosen, and says so under the select", async () => {
    const puts = connect();
    await choose("Escopo do servidor", "Área");
    await choose("Empresa", "Acme");
    await userEvent.setup().click(screen.getByRole("combobox", { name: "Como é alcançado" }));
    await userEvent.setup().click(await screen.findByRole("option", { name: "Endereço remoto (HTTP)" }));
    await fill("crm", "https://crm.example.com/mcp");

    expect(await screen.findByText("Informe a área.")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Área" })).toHaveAttribute("aria-invalid", "true");
    expect(puts()).toHaveLength(0);
  });

  it("sends a company's server with the company alone", async () => {
    const puts = connect();
    await choose("Escopo do servidor", "Empresa");
    await choose("Empresa", "Example");
    expect(screen.queryByRole("combobox", { name: "Área" })).toBeNull();
    await userEvent.setup().click(screen.getByRole("combobox", { name: "Como é alcançado" }));
    await userEvent.setup().click(await screen.findByRole("option", { name: "Endereço remoto (HTTP)" }));
    await fill("crm", "https://crm.example.com/mcp");

    await waitFor(() => expect(puts()).toHaveLength(1));
    expect(puts()[0]?.body).toMatchObject({ scopeKind: "company", company: "example" });
    expect(puts()[0]?.body).not.toHaveProperty("area");
  });

  it("shows an edited server's unregistered scope, locked", async () => {
    connect(undefined, {
      name: "old", transport: "http", url: "https://old.example.com/mcp", enabled: true,
      scopeKind: "area", company: "legacy", area: "gone", editable: true,
    });

    const company = await screen.findByRole("combobox", { name: "Empresa" });
    await waitFor(() => expect(company).toHaveTextContent("legacy (não registrada)"));
    expect(company).toBeDisabled();
    expect(screen.getByRole("combobox", { name: "Área" })).toHaveTextContent("gone (não registrada)");
  });
});
