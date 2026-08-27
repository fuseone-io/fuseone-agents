import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AgentDefinition } from "@/features/agents/agent-definition";

/*
The published definition has two authored parts: prose and declared steps.

They stay distinct because the prose is the body the author wrote and the
steps are the envelopes the Gate uses. The read view still has to name both,
otherwise a published step can change while the definition looks unchanged.
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

  it("includes declared steps in the instruction view", () => {
    render(
      <AgentDefinition
        view="instructions"
        instructions="Diagnosticar alertas recebidos no Slack."
        steps={[{ name: "Teste" }]}
      />,
    );

    expect(
      screen.getByText("Diagnosticar alertas recebidos no Slack."),
    ).toBeInTheDocument();
    expect(screen.getByText("Teste")).toBeInTheDocument();
    expect(
      screen.getByText("não chama nenhuma ferramenta — o agente pensando"),
    ).toBeInTheDocument();
  });
});
