import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { IntegrationCard } from "@/features/admin/integration-card";

const base = { name: "crm", kind: "servidor MCP", description: "bin/devstack mcp" };

describe("a connected system's card", () => {
  it("separates a server switched off from one that is not answering", () => {
    // A screen that painted both red would send somebody to debug a server
    // somebody else switched off on purpose.
    const { rerender } = render(<IntegrationCard {...base} enabled={false} />);
    expect(screen.getByText("desligado")).toBeInTheDocument();

    rerender(
      <IntegrationCard
        {...base}
        enabled
        health={{ reachable: false, toolCount: 0, observedAt: "2026-08-11T12:00:00Z" }}
      />,
    );
    expect(screen.getByText("não responde")).toBeInTheDocument();
  });

  it("says when nobody has tried, rather than implying health", () => {
    // "Never tried" and "tried and failed" are different facts. Collapsing
    // them lets a server nobody connected to look fine.
    render(<IntegrationCard {...base} enabled />);

    expect(screen.getByText("sem contato")).toBeInTheDocument();
    expect(screen.getByText(/Nenhum worker tentou conectar/i)).toBeInTheDocument();
  });

  it("shows why a server failed, in the words the server used", () => {
    // The person reading it is the one who fixes the server.
    render(
      <IntegrationCard
        {...base}
        enabled
        health={{
          reachable: false,
          toolCount: 0,
          observedAt: "2026-08-11T12:00:00Z",
          detail: "exec: bin/devstack: no such file or directory",
        }}
      />,
    );

    expect(screen.getByText(/no such file or directory/)).toBeInTheDocument();
  });

  it("does not report contact for something nothing connects to", () => {
    // A model provider is reached when a run needs it, never on a schedule.
    // "No contact" there would be a false alarm rather than a finding.
    render(<IntegrationCard {...base} kind="openai" enabled observes={false} />);

    expect(screen.queryByText(/Nenhum worker tentou conectar/i)).not.toBeInTheDocument();
    expect(screen.getByText("configurado")).toBeInTheDocument();
  });
});
