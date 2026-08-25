import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { AgentCard } from "@/features/agents/agent-card";
import type { Agent } from "@/features/agents/api";
import type { Tool, ToolEffect } from "@/features/admin/api";

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

function tool(toolId: string, effect: ToolEffect): Tool {
  return {
    toolId,
    server: toolId.split(".")[0] ?? "test",
    effect,
    untrusted: false,
  };
}

describe("an agent card", () => {
  it("opens the agent it describes", () => {
    // The card carries everything but the definition, which is the one thing
    // somebody reading it will want next.
    renderCard(<AgentCard agent={agent()} />);

    expect(
      screen.getByRole("link", { name: /Triagem de chamados/ }),
    ).toHaveAttribute("href", "/agents/triage");
  });

  it("opens the exact version it is showing, not whatever is newest", () => {
    // An old version's card describes that version. Following it to the
    // latest would show a reader text the card was not about.
    renderCard(<AgentCard agent={agent({ latest: false, versionId: "v1" })} />);

    expect(
      screen.getByRole("link", { name: /Triagem de chamados/ }),
    ).toHaveAttribute("href", "/agents/triage?version=v1");
  });

  it("groups the capability pack by integration, so many tools do not stretch the card", () => {
    renderCard(
      <AgentCard
        agent={agent({
          tools: [
            "crm.lookup",
            "crm.reply",
            "crm.close",
            "kb.search",
            "kb.fetch",
            "mail.send",
          ],
        })}
      />,
    );

    const card = screen.getByRole("link", { name: /Triagem de chamados/ });
    expect(card.className).toContain("h-[272px]");
    expect(screen.getByText("crm")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
    expect(screen.getByText("kb")).toBeInTheDocument();
    expect(screen.getByText("+1")).toBeInTheDocument();
    expect(screen.getByText("6 ferramentas")).toBeInTheDocument();
    expect(screen.queryByText("crm.lookup")).not.toBeInTheDocument();
    expect(screen.queryByText("mail")).not.toBeInTheDocument();
  });

  it("uses tool classifications when it can say which packs can write", () => {
    renderCard(
      <AgentCard
        agent={agent({ tools: ["grafana.query", "channel.post"] })}
        catalogue={[
          tool("grafana.query", "read"),
          tool("channel.post", "write"),
        ]}
      />,
    );

    expect(
      screen.getByText("2 ferramentas · 1 pode escrever"),
    ).toBeInTheDocument();
    expect(
      screen.getByTitle("channel — 1 ferramenta, pode escrever").className,
    ).toContain("bg-surface-accent");
    expect(
      screen.getByTitle("grafana — 1 ferramenta somente leitura").className,
    ).toContain("bg-muted");
  });

  it("does not colour an unclassified pack as proven read-only", () => {
    renderCard(
      <AgentCard
        agent={agent({ tools: ["grafana.query", "unknown.exec"] })}
        catalogue={[tool("grafana.query", "read")]}
      />,
    );

    expect(
      screen.getByTitle("grafana — 1 ferramenta somente leitura").className,
    ).toContain("bg-muted");
    expect(screen.getByTitle("unknown.exec").className).toContain(
      "border-dashed",
    );
  });

  it("says so when an agent was granted nothing, rather than showing an empty row", () => {
    renderCard(<AgentCard agent={agent({ tools: [] })} />);
    expect(screen.getByText("Sem ferramentas")).toBeInTheDocument();
  });

  it("leaves run configuration off the card, where the list compares health", () => {
    renderCard(
      <AgentCard
        agent={agent({
          triggers: [{ type: "cron", schedule: "*/15 * * * *" }],
        })}
      />,
    );
    expect(screen.queryByText("cron")).not.toBeInTheDocument();
    expect(screen.queryByText("Teto")).not.toBeInTheDocument();
    expect(screen.queryByText("Gatilhos")).not.toBeInTheDocument();
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
            runs: 4,
            finished: 3,
            waiting: 0,
            costMicros: 20_000,
            lastPhase: "finished",
            lastRunAt: "2026-08-11T12:00:00Z",
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
            runs: 3,
            finished: 1,
            waiting: 2,
            costMicros: 0,
            lastPhase: "awaiting_approval",
            lastRunAt: "2026-08-11T12:00:00Z",
          },
        })}
      />,
    );
    expect(screen.getByText(/2 esperando pessoa/)).toBeInTheDocument();
  });
});
