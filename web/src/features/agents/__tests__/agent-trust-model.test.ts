import { describe, expect, it } from "vitest";
import { agentTrustModel } from "@/features/agents/agent-trust-model";
import type { Agent } from "@/lib/api/client";

describe("agentTrustModel", () => {
  it("keeps a draft in evidence collection when nothing has run", () => {
    const model = agentTrustModel({ agent: agent({ stage: "draft" }) });

    expect(model.status).toBe("needs_evidence");
    expect(model.recommendationKey).toBe("agents.trustRecommendCollect");
    expect(model.evidence.find((item) => item.id === "runs")?.status).toBe(
      "missing",
    );
    expect(model.evidence.find((item) => item.id === "regressions")?.status).toBe(
      "missing",
    );
  });

  it("can recommend autonomous only from measured runs and regression cases", () => {
    const model = agentTrustModel({
      agent: agent({
        stage: "copilot",
        paused: false,
        activity: {
          runs: 12,
          finished: 12,
          waiting: 0,
          costMicros: 10_000,
          lastPhase: "finished",
          lastRunAt: "2026-08-24T12:00:00.000Z",
        },
      }),
      regressions: [{ id: "case-1", expectations: [] }],
    });

    expect(model.status).toBe("ready");
    expect(model.recommendationKey).toBe("agents.trustRecommendAutonomous");
  });

  it("does not punish the agent for waiting on the Gate", () => {
    const model = agentTrustModel({
      agent: agent({
        stage: "autonomous",
        paused: false,
        activity: {
          runs: 4,
          finished: 2,
          waiting: 2,
          costMicros: 20_000,
          lastPhase: "parked",
          lastRunAt: "2026-08-24T12:00:00.000Z",
        },
      }),
      regressions: [{ id: "case-1", expectations: [] }],
    });

    expect(model.status).toBe("needs_evidence");
    expect(model.recommendationKey).toBe("agents.trustRecommendCollect");
    expect(model.evidence.find((item) => item.id === "runs")?.status).toBe(
      "good",
    );
    expect(model.evidence.find((item) => item.id === "runs")?.bodyKey).toBe(
      "agents.trustRunsNoExecutionFailures",
    );
    expect(model.evidence.find((item) => item.id === "decisions")?.status).toBe(
      "unknown",
    );
  });

  it("does not treat a currently running run as bad evidence", () => {
    const model = agentTrustModel({
      agent: agent({
        stage: "copilot",
        paused: false,
        activity: {
          runs: 2,
          finished: 1,
          waiting: 0,
          costMicros: 20_000,
          lastPhase: "running",
          lastRunAt: "2026-08-24T12:00:00.000Z",
        },
      }),
      regressions: [{ id: "case-1", expectations: [] }],
    });

    expect(model.status).toBe("needs_evidence");
    expect(model.evidence.find((item) => item.id === "runs")?.status).toBe(
      "unknown",
    );
  });

  it("does not let a currently running run hide earlier failures", () => {
    const model = agentTrustModel({
      agent: agent({
        stage: "autonomous",
        paused: false,
        activity: {
          runs: 100,
          finished: 0,
          waiting: 0,
          costMicros: 20_000,
          lastPhase: "running",
          lastRunAt: "2026-08-24T12:00:00.000Z",
        },
      }),
      regressions: [{ id: "case-1", expectations: [] }],
    });

    expect(model.status).toBe("needs_review");
    expect(model.recommendationKey).toBe("agents.trustRecommendDemote");
    expect(model.evidence.find((item) => item.id === "runs")?.bodyValues).toMatchObject({
      unfinished: 99,
      runs: 100,
    });
  });
});

function agent(overrides: Partial<Agent>): Agent {
  return {
    agentId: "triage",
    versionId: "v1",
    scope: { company: "acme", area: "platform" },
    name: "Triage",
    provider: "anthropic",
    model: "claude-sonnet-5",
    tools: [],
    budget: { steps: 20 },
    publishedAt: "2026-08-24T10:00:00.000Z",
    latest: true,
    ...overrides,
  };
}
