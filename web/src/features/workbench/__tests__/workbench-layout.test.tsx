import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { setLocale } from "@/i18n";
import { draftOf, openAt, polyfillPointer, serve } from "@/features/workbench/__tests__/workbench-fixtures";

/*
The workbench's layout, as NT-018's mockup draws it: the conversation on the
left, the workspace on the right in tabs, the draft as a summary whose every
part opens its manual controls in a side panel. Nothing is edited in place
and nothing is lost by opening and closing a panel.
*/

const DRAFT = "/agents/triagem/draft";

function world() {
  return serve({
    [`GET ${DRAFT}`]: () => ({ body: draftOf(), etag: '"e1"' }),
    [`GET ${DRAFT}/turns`]: () => ({ body: { items: [], conversation: "open" } }),
  });
}

describe("the workbench layout", () => {
  beforeAll(polyfillPointer);
  beforeEach(() => setLocale("pt-BR"));
  afterEach(() => vi.unstubAllGlobals());

  it("shows the draft as a summary in its tab, beside the conversation", async () => {
    world();
    openAt("/agents/triagem/workbench");
    expect(await screen.findByRole("tab", { name: "Rascunho" })).toHaveAttribute("aria-selected", "true");
    const summary = screen.getByRole("tabpanel", { name: "Rascunho" });
    expect(summary).toHaveTextContent("acme · cx");
    expect(summary).toHaveTextContent("crm.lookup");
    expect(screen.queryByLabelText(/^Nome/)).not.toBeInTheDocument();
    expect(screen.getByLabelText("Mensagem para o assistente")).toBeInTheDocument();
  });

  it("opens a part's manual controls in a side panel, and the summary follows them", async () => {
    world();
    openAt("/agents/triagem/workbench");
    await userEvent.click(await screen.findByRole("button", { name: "Editar identidade e modelo" }));
    const sheet = await screen.findByRole("dialog", { name: "Identidade e modelo" });
    const name = within(sheet).getByLabelText(/^Nome/);
    await userEvent.clear(name);
    await userEvent.type(name, "Triagem nova");
    await userEvent.keyboard("{Escape}");
    expect(screen.getByRole("tabpanel", { name: "Rascunho" })).toHaveTextContent("Triagem nova");
    expect(screen.getByRole("button", { name: /Salvar no rascunho/ })).toBeEnabled();
  });

  it("opens the tools and the governance in their own panels", async () => {
    world();
    openAt("/agents/triagem/workbench");
    await userEvent.click(await screen.findByRole("button", { name: "Editar ferramentas" }));
    expect(await screen.findByRole("dialog", { name: "Ferramentas" })).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    await userEvent.click(screen.getByRole("button", { name: "Editar gatilhos e governança" }));
    expect(await screen.findByRole("dialog", { name: "Gatilhos e governança" })).toBeInTheDocument();
  });

  it("opens the steps in their own panel", async () => {
    world();
    openAt("/agents/triagem/workbench");
    await userEvent.click(await screen.findByRole("button", { name: "Editar etapas" }));
    expect(await screen.findByRole("dialog", { name: "Etapas" })).toBeInTheDocument();
  });

  // The instructions' lint, moved from the old editor: a tool they name that
  // the agent does not hold, enabled in place.
  it("says when the instructions name a tool the agent does not hold, and enables it", async () => {
    serve({
      [`GET ${DRAFT}`]: () => ({ body: draftOf({ instructions: "Objetivo\nUse crm.refund para devolver." }), etag: '"e1"' }),
      [`GET ${DRAFT}/turns`]: () => ({ body: { items: [], conversation: "open" } }),
      "GET /admin/tools": () => ({ body: { items: [{ toolId: "crm.refund", server: "crm", effect: "read", untrusted: false }] } }),
    });
    openAt("/agents/triagem/workbench");
    const summary = await screen.findByRole("tabpanel", { name: "Rascunho" });
    expect(await within(summary).findByText(/Este agente não tem habilitada/)).toBeInTheDocument();
    await userEvent.click(within(summary).getByRole("button", { name: "Habilitar a ferramenta" }));
    expect(screen.getByRole("button", { name: /Salvar no rascunho · 1 alteração/ })).toBeInTheDocument();
  });

  it("says how large the instructions are, as the agent's model counts them", async () => {
    const calls = serve({
      [`GET ${DRAFT}`]: () => ({ body: draftOf(), etag: '"e1"' }),
      [`GET ${DRAFT}/turns`]: () => ({ body: { items: [], conversation: "open" } }),
      "POST /agents/instructions/tokens": () => ({ body: { counted: true, tokens: 1234 } }),
    });
    openAt("/agents/triagem/workbench");
    // Counted once the instructions settle, as the old editor counted them.
    const summary = await screen.findByRole("tabpanel", { name: "Rascunho" });
    await waitFor(() => expect(summary).toHaveTextContent(/1\.?234 tokens/), { timeout: 3000 });
    expect(calls.map((c) => `${c.method} ${c.path}`)).toContain("POST /agents/instructions/tokens");
  });
});
