import { describe, expect, it } from "vitest";
import { parse, serialise } from "@/features/agents/instruction-blocks";

/*
Blocks are a way of writing, not a second format.

What the model receives is text, and what a version stores is that same text.
So the only thing that matters here is that the two directions agree: what
somebody writes as blocks reads back as the same blocks, and an instruction
written before any of this existed survives untouched.
*/

describe("instructions, as blocks", () => {
  it("keeps an instruction nobody structured whole, in one block", () => {
    // Rule six of the handoff, and the one that decides whether anybody
    // adopts this: nobody with a prompt already written is made to
    // restructure it in order to save.
    const written = "Você atende chamados.\n\nSe não encontrar, avise e pare.";

    expect(parse(written)).toEqual([{ kind: "prose", text: written }]);
    expect(serialise(parse(written), "pt-BR")).toBe(written);
  });

  it("reads back what it wrote", () => {
    const blocks = [
      { kind: "objective" as const, text: "Você compara dois registros." },
      { kind: "never" as const, text: "Não invente um cadastro." },
    ];

    expect(parse(serialise(blocks, "pt-BR"))).toEqual(blocks);
  });

  it("recognises a label written in the other language", () => {
    // One definition, two readers: a colleague opening the console in English
    // must not find a Portuguese author's blocks collapsed into a paragraph.
    const written = serialise(
      [{ kind: "whenToStop", text: "Se o valor passar de R$ 500." }],
      "pt-BR",
    );

    expect(parse(written)[0]?.kind).toBe("whenToStop");
  });

  it("puts the label in the language the author is writing in", () => {
    // The label reaches the model with the prose around it, so it is written
    // in the language the prose is written in.
    const blocks = [{ kind: "objective" as const, text: "Compare." }];

    expect(serialise(blocks, "pt-BR")).toBe("Objetivo\nCompare.");
    expect(serialise(blocks, "en-US")).toBe("Purpose\nCompare.");
  });

  it("drops a block somebody left empty", () => {
    const blocks = [
      { kind: "objective" as const, text: "Compare." },
      { kind: "never" as const, text: "   " },
    ];

    expect(serialise(blocks, "pt-BR")).toBe("Objetivo\nCompare.");
  });
});
