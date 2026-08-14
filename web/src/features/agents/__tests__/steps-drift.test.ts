import { describe, expect, it } from "vitest";
import { undescribed } from "@/features/agents/steps-drift";

/*
The drawing and the words are authored separately, so they can disagree.

Which is fine in one direction and not in the other: prose may say more than
the permissions do, and permissions saying more than the prose means an agent
is allowed something nobody wrote down.
*/

describe("what the drawing says and the instructions do not", () => {
  it("names a tool a stage reaches that the text never mentions", () => {
    expect(
      undescribed(
        [{ name: "Pagar", reaches: ["erp.transfer"] }],
        "Responda o cliente e encerre o chamado.",
      ),
    ).toEqual(["erp.transfer"]);
  });

  it("accepts the tool named without its server", () => {
    // Demanding the qualified identifier would fire on every agent whose
    // author writes like a person.
    expect(
      undescribed(
        [{ name: "Encontrar", reaches: ["crm.lookup"] }],
        "Use o lookup para achar o cliente pelo e-mail.",
      ),
    ).toEqual([]);
  });

  it("says nothing about prose that describes more than was drawn", () => {
    // The safe direction: instructions may say more than the permissions do.
    expect(
      undescribed([{ name: "Pensar" }], "Consulte o CRM, responda e encerre."),
    ).toEqual([]);
  });
});
