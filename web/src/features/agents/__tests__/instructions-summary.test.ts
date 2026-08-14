import { describe, expect, it } from "vitest";
import { summarise } from "@/features/agents/instructions-summary";
import type { Tool } from "@/lib/api/client";

/*
Every count on this card is read out of the card.

A number somebody authored is a number that is wrong by the second edit, and
the whole reason to show one here is to be able to trust it at a glance.
*/

const CATALOGUE: Tool[] = [
  { toolId: "crm.lookup", server: "crm", effect: "read", untrusted: true },
  { toolId: "crm.reply", server: "crm", effect: "write", untrusted: true },
];

describe("what an instruction amounts to", () => {
  it("counts a tool once however often the prose names it", () => {
    const found = summarise(
      [
        { kind: "objective", text: "Use crm.lookup." },
        { kind: "howToAct", text: "Depois use crm.lookup de novo e crm.reply." },
      ],
      CATALOGUE,
      [],
      "",
    );

    expect(found.tools).toBe(2);
  });

  it("counts a block that says where it gives up", () => {
    const found = summarise(
      [
        { kind: "whenToStop", text: "Se não encontrar o cliente." },
        // Written and then emptied: it says nothing, so it stops nothing.
        { kind: "whenToStop", text: "  " },
      ],
      CATALOGUE,
      [],
      "",
    );

    expect(found.stops).toBe(1);
  });

  it("measures the payload rather than the blocks", () => {
    // What is counted is what leaves, which is not the sum of the boxes: the
    // labels travel with the prose.
    expect(summarise([], CATALOGUE, [], "Objetivo\nCompare.").characters).toBe(
      "Objetivo\nCompare.".length,
    );
  });
});
