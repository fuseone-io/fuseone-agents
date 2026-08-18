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

  it("wraps long prose inside the instruction column", () => {
    const longWord = "documentacao".repeat(24);
    render(
      <InstructionsRead
        instructions={`Objetivo\nLer ${longWord} antes de responder.`}
      />,
    );

    const prose = screen.getByText(new RegExp(longWord));
    expect(prose).toHaveClass("min-w-0", "break-words");
    expect(prose.closest("div")).toHaveClass("min-w-0");
  });

  it("keeps the labels it has when only some blocks carry one", () => {
    // Half-labelled is the ordinary state of an instruction somebody is part
    // way through structuring, and the half that is labelled has to keep it.
    render(
      <InstructionsRead
        instructions={"Atender chamados.\n\nQuando parar\nSe não achar o cliente."}
      />,
    );

    expect(screen.getByText("Quando parar")).toBeInTheDocument();
    expect(screen.getByText("Atender chamados.")).toBeInTheDocument();
  });

  it("reserves no margin at all when nothing is labelled", () => {
    // Most instructions ever written are unlabelled prose, and they arrive
    // here whole. "No label" beside them is a word that means nothing to a
    // reader — on the editor it is a control, and here there is nothing to
    // press. An empty column where a label would go reads as a defect, so
    // there is no column.
    const { container } = render(
      <InstructionsRead
        instructions={"Atender chamados.\n\nParar se não achar o cliente."}
      />,
    );

    expect(screen.queryByText(/Sem rótulo/)).not.toBeInTheDocument();
    expect(container.querySelectorAll("span")).toHaveLength(0);
  });
});
