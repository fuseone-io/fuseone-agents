import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
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

function renderCard(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

describe("an agent card", () => {
  it("opens the agent it describes", () => {
    // The card carries everything but the definition, which is the one thing
    // somebody reading it will want next.
    renderCard(<AgentCard agent={agent()} />);

    expect(screen.getByRole("link", { name: /Triagem de chamados/ })).toHaveAttribute(
      "href",
      "/agents/triage",
    );
  });

  it("opens the exact version it is showing, not whatever is newest", () => {
    // An old version's card describes that version. Following it to the
    // latest would show a reader text the card was not about.
    renderCard(<AgentCard agent={agent({ latest: false, versionId: "v1" })} />);

    expect(screen.getByRole("link", { name: /Triagem de chamados/ })).toHaveAttribute(
      "href",
      "/agents/triage?version=v1",
    );
  });

  it("shows the capability pack, because what is not there cannot be invoked", () => {
    renderCard(<AgentCard agent={agent({ tools: ["crm.lookup", "kb.search"] })} />);

    expect(screen.getByText("crm.lookup")).toBeInTheDocument();
    expect(screen.getByText("kb.search")).toBeInTheDocument();
  });

  it("says so when an agent was granted nothing, rather than showing an empty row", () => {
    renderCard(<AgentCard agent={agent({ tools: [] })} />);
    expect(screen.getByText("Sem ferramentas")).toBeInTheDocument();
  });

  it("says how a run starts, because an agent nothing triggers never runs", () => {
    renderCard(<AgentCard agent={agent({ triggers: [{ type: "cron", schedule: "*/15 * * * *" }] })} />);
    expect(screen.getByText("cron")).toBeInTheDocument();
  });

  it("calls a manual agent manual instead of leaving the field blank", () => {
    renderCard(<AgentCard agent={agent()} />);
    expect(screen.getByText("manual")).toBeInTheDocument();
  });

  it("marks a superseded version, so history is not mistaken for the present", () => {
    renderCard(<AgentCard agent={agent({ latest: false })} />);
    expect(screen.getByText("versão antiga")).toBeInTheDocument();
  });
});

describe("an agent's activity", () => {
  it("says it never ran, rather than showing a zero that looks measured", () => {
    renderCard(<AgentCard agent={agent()} />);

    expect(screen.getByText("Nunca executou")).toBeInTheDocument();
    // Not "0%": zero is a measurement, and there is nothing to measure.
    expect(screen.getAllByText("—").length).toBeGreaterThan(0);
  });

  it("states the share of concluded runs once there are runs", () => {
    renderCard(
      <AgentCard
        agent={agent({
          activity: {
            runs: 4, finished: 3, waiting: 0, costMicros: 20_000,
            lastPhase: "finished", lastRunAt: "2026-08-11T12:00:00Z",
          },
        })}
      />,
    );
    expect(screen.getByText("75%")).toBeInTheDocument();
  });

  it("surfaces runs waiting on a person, which is what the dot is warning about", () => {
    renderCard(
      <AgentCard
        agent={agent({
          activity: {
            runs: 3, finished: 1, waiting: 2, costMicros: 0,
            lastPhase: "awaiting_approval", lastRunAt: "2026-08-11T12:00:00Z",
          },
        })}
      />,
    );
    expect(screen.getByText(/2 esperando pessoa/)).toBeInTheDocument();
  });
});
