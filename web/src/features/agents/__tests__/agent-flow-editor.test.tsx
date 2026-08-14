import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentFlowEditor } from "@/features/agents/agent-flow-editor";
import type { AgentDefinition, Tool } from "@/lib/api/client";

/*
Drawing the process.

One panel of a fixed height, because a form per stage under the canvas made
the page as long as the process itself. What is edited is whichever card is
selected, and what can be dragged in is this agent's own capability pack —
nothing in the rail is something the agent was not granted, so dragging cannot
widen it.
*/

const draft = (over: Partial<AgentDefinition> = {}): AgentDefinition => ({
  name: "Atendimento",
  area: "cx",
  provider: "openai",
  model: "gpt-test",
  instructions: "Encontre o cliente. Depois responda.",
  tools: ["crm.lookup", "crm.reply"],
  ...over,
});

const CATALOGUE: Tool[] = [
  { toolId: "crm.lookup", server: "crm", effect: "read", untrusted: true },
  { toolId: "crm.reply", server: "crm", effect: "write", untrusted: true },
  // In the catalogue and not in the pack: dragging it in is a grant, and the
  // screen has to say so.
  { toolId: "erp.transfer", server: "erp", effect: "financial", untrusted: true },
];

function renderEditor(over: Partial<AgentDefinition> = {}, patch = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <AgentFlowEditor
        draft={draft(over)}
        patch={patch}
        catalogue={CATALOGUE}
        policies={[]}
      />
    </QueryClientProvider>,
  );
  return patch;
}

describe("drawing an agent's process", () => {
  beforeEach(() => vi.restoreAllMocks());
  afterEach(() => vi.unstubAllGlobals());

  it("shows the whole catalogue, and what every tool does", () => {
    // Dragging in a tool the agent does not hold grants it — the same
    // authority the tools section of this form carries. What it must not be is
    // quiet, so the effect is on every row and not only on the granted ones.
    renderEditor();

    expect(screen.getByText("erp.transfer")).toBeInTheDocument();
    expect(screen.getAllByText(/financeir|financial/i).length).toBeGreaterThan(0);
  });

  it("says what the Gate will do with what a stage reaches", () => {
    // The pack is the ceiling and the stage is the permission, so an author
    // looking at one card wants to know what happens here.
    renderEditor({ steps: [{ name: "Pagar", reaches: ["erp.transfer"] }] });

    fireEvent.click(screen.getByText("Pagar"));

    expect(screen.getByText(/Gate/)).toBeInTheDocument();
  });

  it("offers a stage that calls nothing, which is a real answer", () => {
    // A rail of tools alone would teach that every step is a tool call, and
    // the simplest agent here has a step that only reads and decides.
    renderEditor();

    expect(screen.getByText(/só pensa/)).toBeInTheDocument();
  });

  it("edits one stage at a time rather than all of them at once", async () => {
    const patch = renderEditor({
      steps: [{ name: "Encontrar" }, { name: "Responder" }],
    });

    expect(screen.getByText(/Escolha um card/)).toBeInTheDocument();

    // fireEvent rather than userEvent, and only here: a full pointer sequence
    // reaches d3-drag inside XYFlow, which reads `event.view.document` on
    // mousedown — and jsdom leaves `view` unset. The click is what selection
    // listens to; the rest of the sequence is the environment's gap.
    fireEvent.click(screen.getByText("Encontrar"));

    // The inspector now edits that one, and only that one.
    const name = await screen.findByDisplayValue("Encontrar");
    await userEvent.clear(name);
    await waitFor(() => expect(patch).toHaveBeenCalled());
  });

  it("reads the instructions instead of asking for the process again", async () => {
    let sent: unknown;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) => {
        sent = await input.json();
        return new Response(
          JSON.stringify({ tools: [], steps: [{ name: "Encontrar" }] }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }),
    );
    const patch = renderEditor();

    await userEvent.click(
      screen.getByRole("button", { name: /Ler as instruções/ }),
    );

    await waitFor(() =>
      expect(sent).toEqual({
        steps: "Encontre o cliente. Depois responda.",
      }),
    );
    expect(patch).toHaveBeenCalledWith({ steps: [{ name: "Encontrar" }] });
  });
});
