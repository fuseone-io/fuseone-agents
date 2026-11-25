import { describe, expect, it } from "vitest";
import { unplacedTools } from "@/features/agents/steps-unplaced";

/*
A run of an agent with steps is offered only what some step reaches, so a
granted tool no step reaches never runs. The server refuses to publish it from
the workbench; the console says it while the person edits, before saving.
*/
describe("unplacedTools", () => {
  it("lists each granted tool no step reaches, in the order granted", () => {
    const steps = [{ name: "Consultar", reaches: ["crm.lookup"] }, { name: "Pensar" }];
    expect(unplacedTools(["kb.search", "crm.lookup", "crm.reply"], steps)).toEqual(["kb.search", "crm.reply"]);
  });

  it("lists nothing with no steps: a run keeps its whole pack", () => {
    expect(unplacedTools(["kb.search"], [])).toEqual([]);
  });

  it("never lists a platform tool: the platform offers it whatever the step", () => {
    expect(unplacedTools(["$fuseone.memory.find"], [{ name: "Pensar" }])).toEqual([]);
  });

  // The set is explicit, as the server's is: a typo of a platform tool, or a
  // vendor squatting the sigil, is not the platform's and is listed.
  it("lists a tool that only looks like the platform's", () => {
    expect(unplacedTools(["$fuseone.memory.fnd"], [{ name: "Pensar" }])).toEqual(["$fuseone.memory.fnd"]);
  });
});
