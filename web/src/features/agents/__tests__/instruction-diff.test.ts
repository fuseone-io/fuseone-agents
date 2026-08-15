import { describe, expect, it } from "vitest";
import { diffInstructions } from "@/features/agents/instruction-diff";

/*
What changed in the prose, block by block and word by word.

Publishing writes a version runs will be pinned to, and "instructions changed"
is the one line in the summary somebody cannot check. A character count says
the text moved; it does not say whether a sentence was tightened or a rule was
deleted, which are the two things a reviewer is there to tell apart.
*/

const WAS = [
  "Objetivo",
  "Atender chamados que chegam em suporte@.",
  "",
  "Quando parar",
  "Se não encontrar o cliente.",
].join("\n");

describe("what publishing would change in the prose", () => {
  it("leaves a block nobody touched alone", () => {
    const diff = diffInstructions(WAS, WAS);

    expect(diff.map((block) => block.state)).toEqual(["same", "same"]);
  });

  it("shows the words that went and the words that came", () => {
    const now = WAS.replace("Se não encontrar o cliente.", "Se não achar o cliente.");
    const changed = diffInstructions(WAS, now).find(
      (block) => block.state === "changed",
    );

    expect(changed?.pieces.filter((p) => p.kind === "removed")).toEqual([
      { kind: "removed", text: "encontrar" },
    ]);
    expect(changed?.pieces.filter((p) => p.kind === "added")).toEqual([
      { kind: "added", text: "achar" },
    ]);
  });

  it("does not report every block as changed when one arrives above them", () => {
    // A diff by position calls this "everything moved", which is the diff
    // telling a reviewer to read the whole text again rather than the part
    // that is new.
    const now = ["Nunca", "Prometer prazo.", "", WAS].join("\n");
    const diff = diffInstructions(WAS, now);

    expect(diff.map((block) => block.state)).toEqual(["added", "same", "same"]);
  });

  it("reports a block that was deleted, with its words", () => {
    const now = ["Objetivo", "Atender chamados que chegam em suporte@."].join(
      "\n",
    );
    const gone = diffInstructions(WAS, now).find(
      (block) => block.state === "removed",
    );

    expect(gone?.kind).toBe("whenToStop");
    expect(gone?.pieces.every((p) => p.kind === "removed")).toBe(true);
  });
});
