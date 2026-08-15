import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { InstructionsEditor } from "@/features/agents/instructions-editor";

/*
Adding a block, and the two ways somebody asks for one.

The menu is controlled, because `/` typed in a block has to open it too — and
a controlled menu opens only when the state says so. Held as two pieces of
state it opened for the slash and ignored the button, which is a button that
does nothing.
*/

const TOOLS = { catalogue: [], policies: [], enabled: [] };

describe("adding a block", () => {
  it("opens the menu from the button", async () => {
    render(
      <InstructionsEditor
        instructions="Você atende chamados."
        on={{ change: vi.fn(), enable: vi.fn() }}
        tools={TOOLS}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: /Novo bloco/ }));

    expect(await screen.findByRole("menuitem", { name: /Objetivo/ })).toBeInTheDocument();
  });

  it("adds the kind that was picked", async () => {
    const onChange = vi.fn();
    render(
      <InstructionsEditor
        instructions="Você atende chamados."
        on={{ change: onChange, enable: vi.fn() }}
        tools={TOOLS}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: /Novo bloco/ }));
    await userEvent.click(await screen.findByRole("menuitem", { name: /Nunca/ }));

    // An empty block contributes nothing to the payload, so what changes is
    // the text only once somebody writes in it.
    expect(onChange).toHaveBeenCalledWith("Você atende chamados.");
  });
});

describe("writing in a block that was just added", () => {
  it("shows the block, even though it contributes nothing yet", async () => {
    // An empty block sends nothing, and it still has to exist: somebody who
    // asked for one and saw nothing appear concludes the button is broken.
    render(
      <InstructionsEditor
        instructions="Você atende chamados."
        on={{ change: vi.fn(), enable: vi.fn() }}
        tools={TOOLS}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: /Novo bloco/ }));
    await userEvent.click(await screen.findByRole("menuitem", { name: /Nunca/ }));

    expect(
      await screen.findByRole("textbox", { name: "Nunca" }),
    ).toBeInTheDocument();
  });

  it("keeps what is typed, space by space", async () => {
    // Every keystroke used to be serialised and parsed back, and parsing
    // trims: a trailing space vanished as it was typed.
    const onChange = vi.fn();
    render(
      <InstructionsEditor
        instructions={"Objetivo\nCompare."}
        on={{ change: onChange, enable: vi.fn() }}
        tools={TOOLS}
      />,
    );

    await userEvent.click(screen.getByRole("textbox", { name: "Objetivo" }));
    await userEvent.type(
      screen.getByRole("textbox", { name: "Objetivo" }),
      " Depois responda.",
    );

    expect(screen.getByRole("textbox", { name: "Objetivo" })).toHaveValue(
      "Compare. Depois responda.",
    );
  });
});

/*
What publishing would change in the prose.

The publish summary can only say that the instruction changed; a character
count does not distinguish a sentence tightened from a rule deleted, and
telling those apart is what a reviewer is for.
*/
describe("what changed since the published version", () => {
  it("is not offered when nothing changed", () => {
    render(
      <InstructionsEditor
        instructions={"Objetivo\nCompare."}
        on={{ change: vi.fn(), enable: vi.fn() }}
        tools={TOOLS}
        was={"Objetivo\nCompare."}
      />,
    );

    // A segment that is present and empty teaches people it is never worth
    // pressing, and by then it is the one they needed.
    expect(
      screen.queryByRole("tab", { name: /O que mudou/ }),
    ).not.toBeInTheDocument();
  });

  it("marks the words that went and the words that came", async () => {
    render(
      <InstructionsEditor
        instructions={"Objetivo\nCompare os dois lados."}
        on={{ change: vi.fn(), enable: vi.fn() }}
        tools={TOOLS}
        was={"Objetivo\nCompare os dois pedidos."}
      />,
    );

    await userEvent.click(screen.getByRole("tab", { name: /O que mudou/ }));

    expect(await screen.findByText("pedidos.")).toBeInTheDocument();
    expect(screen.getByText("lados.")).toBeInTheDocument();
  });
});
