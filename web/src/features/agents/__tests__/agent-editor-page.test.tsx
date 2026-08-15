import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Toaster } from "@/components/ui/sonner";
import { AgentEditorPage } from "@/features/agents/agent-editor-page";

/*
Every control on the editor, pressed once.

Four defects reached the lab in one afternoon and all four were the same
shape: a control drawn on screen with nothing behind it — a menu that could
not open, a block that could not appear, a warning rendered twice, a handle
that did not drag. None of them broke a unit test, because each component was
correct on its own; what was wrong was the wiring between them.

So this walks the screen the way somebody does. It asserts what each control
was for rather than that it exists, because "it renders" is exactly the
assertion all four defects passed.

Drag is the one thing left out: jsdom has no drag-and-drop worth the name, and
the two reordering hooks are tested where they are decided.
*/

const AGENT = {
  agent: {
    agentId: "suporte",
    versionId: "vb435fd91",
    scope: { company: "acme", area: "cx" },
    name: "Atendimento",
    provider: "openai",
    model: "devstack",
    tools: ["crm.lookup"],
    budget: { micros: 500_000, steps: 60 },
    triggers: [],
    publishedAt: "2026-08-14T12:00:00Z",
    latest: true,
    stage: "draft",
  },
  instructions: "Você atende chamados que chegam em suporte@.",
  versions: [],
};

const TOOLS = {
  items: [
    {
      toolId: "crm.lookup",
      server: "crm",
      description: "Encontra o cliente",
      effect: "read",
      untrusted: true,
    },
    {
      toolId: "crm.reply",
      server: "crm",
      description: "Responde o chamado",
      effect: "write",
      untrusted: true,
    },
  ],
};

/** Answers whatever the screen asks for, so nothing is left loading. */
function stubApi() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      const url = input instanceof Request ? input.url : String(input);
      const body = url.includes("/admin/tools")
        ? TOOLS
        : url.includes("/agents/suporte")
          ? AGENT
          : { items: [] };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

function openEditor() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/agents/suporte/edit"]}>
        <Routes>
          <Route path="/agents/:agentId/edit" element={<AgentEditorPage />} />
        </Routes>
      </MemoryRouter>
      <Toaster />
    </QueryClientProvider>,
  );
}

const tab = (name: RegExp) => screen.getByRole("tab", { name });

describe("the agent editor, control by control", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    stubApi();
  });
  afterEach(() => vi.unstubAllGlobals());

  it("opens each tab, and each says what it decides", async () => {
    openEditor();

    await screen.findByRole("tab", { name: /Definição/ });

    for (const [name, hint] of [
      [/Passos/, /mesma sequência/],
      [/Ferramentas/, /catálogo é da organização/],
      [/Governança/, /Quem dispara/],
      [/Definição/, /Quem é o agente/],
    ] as const) {
      await userEvent.click(tab(name));
      // In the tab bar, which is the thing under test: the same phrase can
      // appear in the tab's own content and mean something else there.
      await waitFor(() =>
        expect(
          within(
            screen.getByRole("tablist", { name: /Seções do editor/ }),
          ).getByText(hint),
        ).toBeInTheDocument(),
      );
    }
  });

  it("adds a block, and keeps what is written in it", async () => {
    openEditor();

    await userEvent.click(
      await screen.findByRole("button", { name: /Novo bloco/ }),
    );
    await userEvent.click(
      await screen.findByRole("menuitem", { name: /Quando parar/ }),
    );

    const box = await screen.findByRole("textbox", { name: "Quando parar" });
    await userEvent.click(box);
    await userEvent.type(
      screen.getByRole("textbox", { name: "Quando parar" }),
      "Se não encontrar.",
    );

    expect(screen.getByRole("textbox", { name: "Quando parar" })).toHaveValue(
      "Se não encontrar.",
    );
  });

  it("adds a step and switches between the two views of it", async () => {
    openEditor();
    await userEvent.click(await screen.findByRole("tab", { name: /Passos/ }));

    await userEvent.click(screen.getByRole("button", { name: /^Passo$/ }));
    expect(await screen.findByRole("button", { name: /Editar o passo 1/ })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: /Fluxo/ }));
    // Adding one selects it, which is what the inspector is for.
    expect(await screen.findByText(/Passo 1 de 1/)).toBeInTheDocument();
  });

  it("grants a tool from the catalogue", async () => {
    openEditor();
    await userEvent.click(await screen.findByRole("tab", { name: /Ferramentas/ }));

    const row = await screen.findByRole("checkbox", { name: /Conceder crm\.reply/ });
    await userEvent.click(row);

    // The tab's own count is the fact: membership changed.
    await waitFor(() =>
      expect(
        within(screen.getByRole("tab", { name: /Ferramentas/ })).getByText("2"),
      ).toBeInTheDocument(),
    );
  });

  it("says what publishing would change, when asked", async () => {
    openEditor();

    await userEvent.click(
      await screen.findByRole("button", { name: /Novo bloco/ }),
    );
    await userEvent.click(await screen.findByRole("menuitem", { name: /Nunca/ }));
    const box = await screen.findByRole("textbox", { name: "Nunca" });
    await userEvent.click(box);
    await userEvent.type(
      screen.getByRole("textbox", { name: "Nunca" }),
      "Não invente um cadastro.",
    );

    await userEvent.click(
      await screen.findByRole("button", { name: /alteração sem publicar/ }),
    );

    expect(await screen.findByText(/Instruções/)).toBeInTheDocument();
  });
});
