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

const TOOLS = { catalogue: [], policies: [] };

describe("adding a block", () => {
  it("opens the menu from the button", async () => {
    render(
      <InstructionsEditor
        instructions="Você atende chamados."
        onChange={vi.fn()}
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
        onChange={onChange}
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
        onChange={vi.fn()}
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
        onChange={onChange}
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
