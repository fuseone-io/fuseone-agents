import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentStepsView } from "@/features/agents/agent-steps-view";
import type { AgentDefinition } from "@/lib/api/client";

/*
The stages, read out of the instructions rather than typed beside them.

Written by hand it was two descriptions of one process, and two descriptions
drift: the prose says one thing, the fields say another, and nobody can tell
which is true. The instructions stay the single account.

Not derived silently either. The prose is instruction to a model and a step is
a permission, so what comes back is a proposal a person corrects — and what
they leave on screen is what gets published.
*/

const draft = (over: Partial<AgentDefinition> = {}): AgentDefinition => ({
  name: "Atendimento",
  area: "cx",
  provider: "openai",
  model: "gpt-test",
  instructions: "Encontre o cliente pelo e-mail. Depois responda o chamado.",
  tools: ["crm.lookup", "crm.reply"],
  ...over,
});

function renderSection(over: Partial<AgentDefinition> = {}, patch = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <AgentStepsView draft={draft(over)} patch={patch} />
    </QueryClientProvider>,
  );
  return patch;
}

describe("the steps of an agent", () => {
  beforeEach(() => vi.restoreAllMocks());
  afterEach(() => vi.unstubAllGlobals());

  it("says what declaring none actually means", () => {
    // Not an empty state waiting to be filled: one envelope holding the whole
    // pack is the common and correct answer, and a different thing from a
    // single step.
    renderSection();

    expect(screen.getByText(/envelope só/)).toBeInTheDocument();
  });

  it("reads the instructions instead of asking for them again", async () => {
    let sent: unknown;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) => {
        sent = await input.json();
        return new Response(
          JSON.stringify({
            tools: ["crm.lookup"],
            steps: [{ name: "Encontrar o cliente", reaches: ["crm.lookup"] }],
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }),
    );
    const patch = renderSection();

    await userEvent.click(
      screen.getByRole("button", { name: /Ler as instruções/ }),
    );

    // The author's own words, not a second questionnaire.
    await waitFor(() =>
      expect(sent).toEqual({
        steps: "Encontre o cliente pelo e-mail. Depois responda o chamado.",
      }),
    );
    expect(patch).toHaveBeenCalledWith({
      steps: [{ name: "Encontrar o cliente", reaches: ["crm.lookup"] }],
    });
  });

  it("cannot read instructions that are not written yet", () => {
    renderSection({ instructions: "   " });

    expect(
      screen.getByRole("button", { name: /Ler as instruções/ }),
    ).toBeDisabled();
  });

  it("narrows a step to tools the agent already holds", async () => {
    const patch = renderSection({ steps: [{ name: "Encontrar" }] });

    // Chosen from the pack rather than typed: a step naming a tool the agent
    // does not hold is a permission that reads as granted and is refused.
    await userEvent.click(screen.getByRole("button", { name: /crm.lookup/ }));

    expect(patch).toHaveBeenCalledWith({
      steps: [{ name: "Encontrar", reaches: ["crm.lookup"] }],
    });
  });
});
