import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { AgentCard } from "@/features/agents/agent-card";
import type { Agent } from "@/features/agents/api";

const agent = (over: Partial<Agent> = {}): Agent => ({
  agentId: "triage",
  versionId: "v3f9a1c2b8d40",
  scope: { company: "acme", area: "cx" },
  name: "Triagem de chamados",
  provider: "openai",
  model: "gpt-test",
  tools: ["crm.lookup"],
  budget: { micros: 500_000, steps: 60 },
  publishedAt: "2026-08-11T12:00:00Z",
  latest: true,
  ...over,
});

describe("an agent card", () => {
  it("shows the capability pack, because what is not there cannot be invoked", () => {
    render(<AgentCard agent={agent({ tools: ["crm.lookup", "kb.search"] })} />);

    expect(screen.getByText("crm.lookup")).toBeInTheDocument();
    expect(screen.getByText("kb.search")).toBeInTheDocument();
  });

  it("says so when an agent was granted nothing, rather than showing an empty row", () => {
    render(<AgentCard agent={agent({ tools: [] })} />);
    expect(screen.getByText("Sem ferramentas")).toBeInTheDocument();
  });

  it("says how a run starts, because an agent nothing triggers never runs", () => {
    render(<AgentCard agent={agent({ triggers: [{ type: "cron", schedule: "*/15 * * * *" }] })} />);
    expect(screen.getByText("cron")).toBeInTheDocument();
  });

  it("calls a manual agent manual instead of leaving the field blank", () => {
    render(<AgentCard agent={agent()} />);
    expect(screen.getByText("manual")).toBeInTheDocument();
  });

  it("marks a superseded version, so history is not mistaken for the present", () => {
    render(<AgentCard agent={agent({ latest: false })} />);
    expect(screen.getByText("versão antiga")).toBeInTheDocument();
  });
});
