import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ServerEditor } from "@/features/integrations/server-editor";
import { ScopeChoiceContext } from "@/features/integrations/scope/scope-choice";
import { setLocale } from "@/i18n";
import { json, stubNetwork } from "@/test/revised-routes";

// NT-019: what the server refuses about an area's new server is said beside
// the field it is about, in the reader's language.

function refusedWith(status: number, problem: Record<string, unknown>) {
  stubNetwork([], (method) => (method === "PUT" ? json({ status, ...problem }, status) : undefined));
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ScopeChoiceContext.Provider value={{ anyScope: false, configure: [{ company: "acme", area: "cx" }] }}>
        <ServerEditor server={null} onClose={vi.fn()} />
      </ScopeChoiceContext.Provider>
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
    refusedWith(400, {
      type: "fuseone:invalid-input", title: "Invalid request",
      detail: "admin: outside the platform's egress policy for this server's scope: https://crm.example.com",
    });
    await save();

    expect(await screen.findByText(/fora da política de egresso da plataforma/)).toHaveTextContent(
      "https://crm.example.com",
    );
  });
});
