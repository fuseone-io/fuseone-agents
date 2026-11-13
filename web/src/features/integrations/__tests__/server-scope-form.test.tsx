import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Toaster } from "@/components/ui/sonner";
import { ServerEditor } from "@/features/integrations/server-editor";
import { ScopeChoiceContext, type ScopeChoice } from "@/features/integrations/scope/scope-choice";
import { setLocale } from "@/i18n";
import { json, revisedCollection, stubNetwork } from "@/test/revised-routes";

// NT-019: a new server is placed at a scope, and outside the installation it
// is remote, because a local one runs on the platform's worker.

const areaAdmin: ScopeChoice = { anyScope: false, configure: [{ company: "acme", area: "cx" }] };

function connect(choice?: ScopeChoice) {
  const servers = revisedCollection<{ name: string }>("/admin/integrations/mcp-servers", []);
  const requests = stubNetwork([servers], (_m, path) =>
    path === "/admin/integrations" ? json({ mcpServers: [], providers: [] }) : undefined,
  );
  const editor = <ServerEditor server={null} onClose={vi.fn()} />;
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
});
