import { describe, expect, it } from "vitest";
import { BLANK } from "@/features/agents/agent-draft";
import { agentRequirements } from "@/features/agents/agent-required";
import {
  guidedAgentProgress,
  guidedAgentSteps,
  publishedAgentGuideSteps,
} from "@/features/agents/agent-guided-path-model";
import type { Agent, AgentDefinition, Tool } from "@/lib/api/client";
import type { MCPUserCredential } from "@/features/integrations/api";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
import type { components } from "@/lib/api/schema.gen";

describe("the guided first-agent path", () => {
  it("groups the publish requirements instead of inventing another list", () => {
    const steps = guidedAgentSteps(agentRequirements("", BLANK), BLANK);

    expect(steps.map((step) => [step.id, step.done])).toEqual([
      ["identity", false],
      ["instructions", false],
      ["steps", false],
      ["tools", false],
      ["governance", true],
      ["simulation", false],
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

  it("keeps an unclassified selected tool out of the ready path", () => {
    const draft = publishableDraft({ tools: ["outline.fetch"] });
    const steps = guidedAgentSteps(agentRequirements("reader", draft), draft, {
      catalogue: [tool("outline.fetch", { effect: "unknown" })],
    });

    expect(steps.find((step) => step.id === "tools")).toMatchObject({
      done: false,
      bodyKey: "agents.guideToolsClassifyHint",
    });
  });

  it("names missing personal MCP credentials only when recipe and credential facts prove it", () => {
    const draft = publishableDraft({ tools: ["google.search"] });
    const recipe: ServerRecipe = {
      server: "google",
      title: "Google",
      category: "knowledge",
      publisher: "Google",
      docsFrom: "publisher",
      provenance: "documentation",
      status: "published",
      configRequirements: ["credential"],
      requiresPersonalCredential: true,
      transport: "http",
      authModes: [{ type: "oauth2", principal: "user" }],
    };
    const steps = guidedAgentSteps(agentRequirements("reader", draft), draft, {
      catalogue: [tool("google.search", { server: "google" })],
      recipes: [recipe],
      credentials: [],
    });

    expect(steps.find((step) => step.id === "tools")).toMatchObject({
      done: false,
      bodyKey: "agents.guideToolsPersonalCredentialHint",
    });

    const credential: MCPUserCredential = {
      server: "google",
      hasCredential: true,
      hasHeaders: false,
      hasOAuth: true,
    };
    const ready = guidedAgentSteps(agentRequirements("reader", draft), draft, {
      catalogue: [tool("google.search", { server: "google" })],
      recipes: [recipe],
      credentials: [credential],
    });

    expect(ready.find((step) => step.id === "tools")).toMatchObject({
      done: true,
      bodyKey: "agents.guideToolsReadyHint",
    });
  });

  it("checks that a channel trigger has a conversation in the same scope", () => {
    const draft = publishableDraft({
      triggers: [{ type: "channel" }],
      company: "cora",
      area: "platform",
    });
    const wrongScope = channel({
      scope: { company: "default", area: "platform" },
    });

    const blocked = guidedAgentSteps(agentRequirements("sre", draft), draft, {
      agentId: "sre",
      channels: [wrongScope],
    });
    expect(blocked.find((step) => step.id === "governance")).toMatchObject({
      done: false,
      bodyKey: "agents.guideGovernanceNoChannelHint",
    });

    const ready = guidedAgentSteps(agentRequirements("sre", draft), draft, {
      agentId: "sre",
      channels: [
        channel({
          scope: { company: "cora", area: "platform" },
          mode: "mentions",
        }),
      ],
    });
    expect(ready.find((step) => step.id === "governance")).toMatchObject({
      done: true,
      bodyKey: "agents.guideGovernanceChannelHint",
    });
  });

  it("turns a published paused agent into a launch check instead of a hidden precondition", () => {
    const steps = publishedAgentGuideSteps(agent({ paused: true }), "Do work.", {
      catalogue: [tool("github.list_issues")],
    });

    expect(steps.map((step) => [step.id, step.done, step.optional])).toEqual([
      ["tools", true, undefined],
      ["governance", true, undefined],
      ["simulation", false, true],
      ["launch", false, undefined],
    ]);
  });
});

function publishableDraft(
  overrides: Partial<AgentDefinition> = {},
): AgentDefinition {
  return {
    ...BLANK,
    name: "Ticket triage",
    company: "acme",
    area: "support",
    provider: "anthropic",
    model: "claude-opus-5",
    instructions: "Read the ticket and answer only with what is known.",
    tools: ["github.list_issues"],
    budget: { steps: 60 },
    ...overrides,
  };
}

function tool(id: string, overrides: Partial<Tool> = {}): Tool {
  return {
    toolId: id,
    server: id.split(".")[0] ?? id,
    effect: "read",
    untrusted: false,
    onSurface: true,
    offered: true,
    ...overrides,
  };
}

function channel(
  conversation: Partial<components["schemas"]["ChannelConversation"]>,
): components["schemas"]["Channel"] {
  return {
    name: "cora-slack",
    kind: "slack",
    enabled: true,
    hasCredential: true,
    conversations: [
      {
        id: "C01",
        scope: { company: "cora", area: "platform" },
        enabled: true,
        ...conversation,
      },
    ],
  };
}

function agent(overrides: Partial<Agent> = {}): Agent {
  return {
    agentId: "github",
    versionId: "v123",
    scope: { company: "acme", area: "support" },
    name: "GitHub",
    provider: "anthropic",
    model: "claude-opus-5",
    tools: ["github.list_issues"],
    budget: { steps: 60 },
    triggers: [],
    publishedAt: "2026-08-20T12:00:00Z",
    latest: true,
    ...overrides,
  };
}
