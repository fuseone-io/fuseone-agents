import { describe, expect, it } from "vitest";
import {
  operationsFor,
  rebase,
  type Field,
} from "@/features/workbench/draft-operations";
import type { AgentDefinition } from "@/lib/api/client";

/*
What the person changed in the panel, as the operations the draft takes
(NT-018 1b, contract 1): one per field, only for fields they touched and that
differ, and never the instructions, which change only through the
conversation.
*/

const BASE: AgentDefinition = {
  name: "Triagem",
  company: "acme",
  area: "cx",
  provider: "openai",
  model: "m",
  instructions: "Objetivo\nTriar.",
  tools: ["crm.lookup"],
  budget: { micros: 500_000, steps: 60 },
  triggers: [],
  steps: [{ name: "Ler", reaches: ["crm.lookup"] }],
};

const touched = (...fields: Field[]) => new Set<Field>(fields);

describe("operationsFor", () => {
  it("sends nothing for a field touched and put back", () => {
    expect(operationsFor(BASE, { ...BASE }, touched("name", "tools"))).toEqual([]);
  });

  it("never sends the instructions, however they differ", () => {
    const after = { ...BASE, instructions: "Outra coisa." };
    expect(operationsFor(BASE, after, touched("instructions"))).toEqual([]);
  });

  it("sends one identity operation for the name and the scope together", () => {
    const after = { ...BASE, name: "Triagem ops", area: "ops" };
    expect(operationsFor(BASE, after, touched("name", "area"))).toEqual([
      { kind: "set_identity", name: "Triagem ops", company: "acme", area: "ops" },
    ]);
  });

  it("adds before it removes, so a step never loses its tool midway", () => {
    const after = { ...BASE, tools: ["crm.reply"] };
    expect(operationsFor(BASE, after, touched("tools"))).toEqual([
      { kind: "add_tool", tool: "crm.reply" },
      { kind: "remove_tool", tool: "crm.lookup" },
    ]);
  });

  it("reads steps by name: added, edited and removed", () => {
    const after = {
      ...BASE,
      steps: [{ name: "Ler", reaches: [] }, { name: "Responder" }],
    };
    expect(operationsFor(BASE, after, touched("steps"))).toEqual([
      { kind: "edit_step", stepName: "Ler", step: { name: "Ler", reaches: [] } },
      { kind: "add_step", step: { name: "Responder" } },
    ]);
  });

  it("sends a whole-field operation for the model and the budget", () => {
    const after = { ...BASE, model: "m2", budget: { micros: 1_000_000, steps: 60 } };
    expect(operationsFor(BASE, after, touched("model", "budget"))).toEqual([
      { kind: "set_model", provider: "openai", model: "m2" },
      { kind: "set_budget", budget: { micros: 1_000_000, steps: 60 } },
    ]);
  });

  it("ignores a field that differs but nobody touched", () => {
    const after = { ...BASE, replyLocale: "en-US" };
    expect(operationsFor(BASE, after, touched())).toEqual([]);
  });
});

describe("rebase", () => {
  it("never takes the instructions from the panel, even marked as touched", () => {
    const local = { ...BASE, instructions: "Escrito à mão." };
    expect(rebase(BASE, local, touched("instructions")).instructions).toBe(BASE.instructions);
  });

  it("keeps what the person touched over a draft read again", () => {
    const server = { ...BASE, model: "newer", replyLocale: "pt-BR" };
    const local = { ...BASE, replyLocale: "en-US", model: "local-untouched" };
    const out = rebase(server, local, touched("replyLocale"));
    expect(out.replyLocale).toBe("en-US");
    expect(out.model).toBe("newer");
  });
});
