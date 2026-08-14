import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { AgentStepsSection } from "@/features/agents/agent-steps-section";
import type { AgentDefinition } from "@/lib/api/client";

/*
Declaring the process.

The stages are what the Gate is meant to obey — `reaches` is the permission
while a run sits at one — and until now there was nowhere in the console to
write one down. An agent authored here could only ever have a single envelope
holding its whole capability pack.
*/

const draft = (over: Partial<AgentDefinition> = {}): AgentDefinition => ({
  name: "Atendimento",
  area: "cx",
  provider: "openai",
  model: "gpt-test",
  instructions: "Responda o cliente.",
  tools: ["crm.lookup", "crm.reply"],
  ...over,
});

describe("declaring the steps of an agent", () => {
  it("says what declaring none actually means", () => {
    // Not an empty state to be filled in: one envelope holding the whole pack
    // is the common and correct answer, and it is a different thing from a
    // single step.
    render(<AgentStepsSection draft={draft()} patch={vi.fn()} />);

    expect(screen.getByText(/envelope só/)).toBeInTheDocument();
  });

  it("narrows a step to tools the agent already holds", async () => {
    const patch = vi.fn();
    render(
      <AgentStepsSection
        draft={draft({ steps: [{ name: "Encontrar" }] })}
        patch={patch}
      />,
    );

    // Chosen from the pack rather than typed: a step naming a tool the agent
    // does not hold is a permission that reads as granted and is refused.
    await userEvent.click(screen.getByRole("button", { name: /crm.lookup/ }));

    expect(patch).toHaveBeenCalledWith({
      steps: [{ name: "Encontrar", reaches: ["crm.lookup"] }],
    });
  });
});
