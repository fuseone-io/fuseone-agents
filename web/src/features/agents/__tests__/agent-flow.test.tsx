import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AgentDefinition } from "@/features/agents/agent-definition";

/*
The steps are not a prettier rendering of the prose.

The prose is what the model reads; the steps are what the Gate is meant to
obey — `reaches` is the permission while a run sits at one, narrower than the
capability pack. Showing them as one document would hide that they are two
things with two readers.
*/

describe("an agent's definition", () => {
  it("offers the steps only when the version declares some", () => {
    // One envelope holding the whole pack is not one step, and a diagram of
    // it would teach a reader the opposite.
    render(<AgentDefinition instructions="Responda o cliente." />);

    expect(screen.queryByRole("tab", { name: "Passos" })).not.toBeInTheDocument();
    expect(screen.getByText("Responda o cliente.")).toBeInTheDocument();
  });

  it("names the tools each step may reach", () => {
    render(
      <AgentDefinition
        instructions="Responda o cliente."
        steps={[
          { name: "Encontrar o cliente", reaches: ["crm.lookup"] },
          { name: "Responder", reaches: ["crm.reply"] },
        ]}
      />,
    );

    expect(screen.getByRole("tab", { name: "Passos" })).toBeInTheDocument();
  });

  it("says a step that calls nothing is the agent thinking", () => {
    // An empty box is the point rather than a gap: a step is not a tool call,
    // and a model where it was could not describe the simplest agent here.
    render(
      <AgentDefinition
        instructions="Pense."
        steps={[{ name: "Resumir o caso" }]}
      />,
    );

    expect(screen.getByRole("tab", { name: "Passos" })).toBeInTheDocument();
  });
});
