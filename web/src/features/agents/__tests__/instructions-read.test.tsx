import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { InstructionsRead } from "@/features/agents/instructions-read";

/*
The published instruction, read the way it was written.

The editor gives a prompt's own hierarchy a margin; a version read as one
paragraph throws that away at exactly the moment somebody is trying to
understand what an agent was told. Same structure, no controls: a version is
changed by publishing another one.
*/

describe("a published instruction", () => {
  it("puts each block's label in the margin", () => {
    render(
      <InstructionsRead
        instructions={"Objetivo\nAtender chamados.\n\nQuando parar\nSe não achar o cliente."}
      />,
    );

    expect(screen.getByText("Objetivo")).toBeInTheDocument();
    expect(screen.getByText("Quando parar")).toBeInTheDocument();
    expect(screen.getByText("Atender chamados.")).toBeInTheDocument();
  });

  it("leaves the margin empty for prose nobody labelled", () => {
    // Most instructions ever written are one unlabelled paragraph. "No label"
    // beside it is a word that means nothing to a reader — on the editor it
    // is a control, and here there is nothing to press.
    render(<InstructionsRead instructions="Atender chamados que chegam em suporte@." />);

    expect(screen.queryByText(/Sem rótulo/)).not.toBeInTheDocument();
    expect(
      screen.getByText("Atender chamados que chegam em suporte@."),
    ).toBeInTheDocument();
  });
});
