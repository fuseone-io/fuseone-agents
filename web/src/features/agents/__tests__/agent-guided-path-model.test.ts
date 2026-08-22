import { describe, expect, it } from "vitest";
import { BLANK } from "@/features/agents/agent-draft";
import { agentRequirements } from "@/features/agents/agent-required";
import {
  guidedAgentProgress,
  guidedAgentSteps,
} from "@/features/agents/agent-guided-path-model";
import type { AgentDefinition } from "@/lib/api/client";

describe("the guided first-agent path", () => {
  it("groups the publish requirements instead of inventing another list", () => {
    const steps = guidedAgentSteps(agentRequirements("", BLANK), BLANK);

    expect(steps.map((step) => [step.id, step.done])).toEqual([
      ["identity", false],
      ["instructions", false],
      ["steps", false],
      ["tools", false],
      ["governance", true],
      ["publish", false],
    ]);
    expect(guidedAgentProgress(steps)).toMatchObject({
      done: 1,
      total: 5,
      next: { id: "identity" },
    });
  });

  it("treats reviewed stages as recommended, not a publishing blocker", () => {
    const draft: AgentDefinition = {
      ...BLANK,
      name: "Ticket triage",
      company: "acme",
      area: "support",
      provider: "anthropic",
      model: "claude-opus-5",
      instructions: "Read the ticket and answer only with what is known.",
      tools: ["github.list_issues"],
    };
    const steps = guidedAgentSteps(
      agentRequirements("ticket-triage", draft),
      draft,
    );

    expect(steps.find((step) => step.id === "steps")).toMatchObject({
      done: false,
      optional: true,
    });
    expect(steps.find((step) => step.id === "publish")).toMatchObject({
      done: true,
    });
  });

  it("keeps every required guide step anchored to a real publish requirement", () => {
    const draft: AgentDefinition = {
      ...BLANK,
      name: "Ticket triage",
      company: "acme",
      area: "support",
      provider: "anthropic",
      model: "claude-opus-5",
      instructions: "Read the ticket and answer only with what is known.",
      tools: ["github.list_issues"],
      steps: [{ name: "Read", reaches: ["github.list_issues"] }],
    };
    const steps = guidedAgentSteps(
      agentRequirements("ticket-triage", draft),
      draft,
    );

    expect(
      steps.filter((step) => !step.optional).map((step) => [step.id, step.done]),
    ).toEqual([
      ["identity", true],
      ["instructions", true],
      ["tools", true],
      ["governance", true],
      ["publish", true],
    ]);
  });
});
