import { describe, expect, it } from "vitest";
import { narrate } from "@/features/agents/narrative";
import type { AgentDefinition, Tool } from "@/lib/api/client";

const tools = [
  { toolId: "crm.lookup", effect: "read" },
  { toolId: "crm.reply", effect: "write" },
  { toolId: "crm.purge", effect: "destructive" },
] as Tool[];

const draft = (over: Partial<AgentDefinition> = {}): AgentDefinition =>
  ({
    name: "Suporte", company: "acme", area: "cx", provider: "anthropic", model: "m",
    instructions: "", tools: [], budget: { micros: 500_000, steps: 60 },
    triggers: [], ...over,
  }) as AgentDefinition;

describe("reading back what the platform understood", () => {
  it("says who it asks before acting, derived from the Gate rather than from a setting", () => {
    const lines = narrate(draft({ tools: ["crm.lookup", "crm.reply"] }), tools, []);
    expect(lines).toContainEqual({ key: "narrative.asks", values: { tools: "crm.reply" } });
  });

  it("says plainly when nothing will stop for a person", () => {
    // Silence would read as "nothing to worry about". An agent that can act
    // without asking anybody is the case an approver most needs told.
    const lines = narrate(draft({ tools: ["crm.lookup"] }), tools, []);
    expect(lines).toContainEqual({ key: "narrative.asksNobody" });
  });

  it("names what it will never be allowed to do", () => {
    const lines = narrate(draft({ tools: ["crm.purge"] }), tools, []);
    expect(lines).toContainEqual({ key: "narrative.never", values: { tools: "crm.purge" } });
  });

  it("describes the ceiling as what happens, not as a number nobody reads", () => {
    const lines = narrate(draft(), tools, []);
    expect(lines.at(-1)).toEqual({ key: "narrative.boundedBoth", values: { micros: 500_000, steps: 60 } });
  });

  it("says an agent nothing triggers only runs when somebody asks", () => {
    expect(narrate(draft(), tools, [])[0]).toEqual({ key: "narrative.startedByHand" });
  });
});
