import { describe, expect, it } from "vitest";
import { BLANK } from "@/features/agents/agent-draft";
import {
  agentRequirementMarked,
  agentRequirements,
} from "@/features/agents/agent-required";
import type { AgentDefinition } from "@/lib/api/client";

describe("agent publish requirements", () => {
  it("names the fields the server cannot invent", () => {
    const missing = agentRequirements("", BLANK).filter((item) => !item.done);

    expect(missing.map((item) => item.id)).toEqual([
      "identifier",
      "name",
      "area",
      "provider",
      "model",
      "instructions",
      "tools",
    ]);
    expect(missing.every((item) => agentRequirementMarked(item.id))).toBe(true);
  });

  it("is empty when the definition can be published", () => {
    const draft: AgentDefinition = {
      ...BLANK,
      name: "Ticket triage",
      area: "support",
      provider: "anthropic",
      model: "claude-opus-5",
      instructions: "Read the ticket and answer only with what is known.",
      tools: ["github.list_issues"],
    };

    expect(
      agentRequirements("ticket-triage", draft).filter((item) => !item.done),
    ).toEqual([]);
  });
});
