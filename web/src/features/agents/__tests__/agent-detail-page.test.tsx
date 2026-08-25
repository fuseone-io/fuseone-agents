import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AgentDetailPage } from "@/features/agents/agent-detail-page";
import { setLocale } from "@/i18n";
import type { AgentDetail, AgentTrust } from "@/lib/api/client";

const detail = vi.hoisted(() => ({
  data: undefined as AgentDetail | undefined,
  trust: undefined as AgentTrust | undefined,
}));

vi.mock("@/features/agents/agent-detail-api", () => ({
  useAgent: () => ({
    data: detail.data,
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  }),
  useAgentTrust: () => ({
    data: detail.trust,
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  }),
}));

vi.mock("@/features/admin/api", () => ({
  useTools: () => ({
    data: {
      items: [
        {
          toolId: "grafana.query_loki_logs",
          server: "grafana",
          effect: "read",
          untrusted: true,
          onSurface: true,
          offered: true,
        },
      ],
    },
  }),
}));

vi.mock("@/features/channels/api", () => ({
  useChannels: () => ({ data: { items: [] } }),
}));

vi.mock("@/features/integrations/api", () => ({
  useMCPUserCredentials: () => ({ data: { items: [] } }),
}));

vi.mock("@/features/integrations/mcp/api", () => ({
  useRecipes: () => ({ data: { items: [] } }),
}));

vi.mock("@/features/agents/agent-primary", () => ({
  AgentPrimary: () => <button type="button">Run</button>,
}));

vi.mock("@/features/agents/agent-more-menu", () => ({
  AgentMoreMenu: () => <button type="button" aria-label="More actions" />,
}));

vi.mock("@/features/agents/stage-control", () => ({
  StageControl: () => <div role="group" aria-label="Trust" />,
}));

vi.mock("@/features/agents/agent-runs", () => ({
  AgentRuns: ({ showHeader }: { showHeader?: boolean }) => (
    <section aria-label="runs panel" data-header={showHeader ? "yes" : "no"} />
  ),
}));

vi.mock("@/features/agents/agent-capabilities", () => ({
  AgentCapabilities: ({ compact }: { compact?: boolean }) => (
    <aside data-testid="capability-rail" data-compact={compact ? "yes" : "no"} />
  ),
}));

vi.mock("@/features/agents/webhooks-panel", () => ({
  WebhooksPanel: () => <aside data-testid="webhooks-rail" />,
}));

vi.mock("@/features/agents/agent-versions", () => ({
  AgentVersions: ({ compact }: { compact?: boolean }) => (
    <aside data-testid="versions-rail" data-compact={compact ? "yes" : "no"} />
  ),
}));

function showDetail() {
  return render(
    <MemoryRouter initialEntries={["/agents/troubleshooting-devops"]}>
      <Routes>
        <Route path="/agents/:agentId" element={<AgentDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("the agent overview", () => {
  beforeEach(() => {
    setLocale("en-US");
    detail.data = {
      agent: {
        agentId: "troubleshooting-devops",
        versionId: "vb6148c24",
        scope: { company: "acme", area: "platform" },
        name: "Troubleshooting DevOps",
        provider: "anthropic",
        model: "claude-opus-5",
        tools: ["grafana.query_loki_logs"],
        budget: { micros: 500_000, steps: 60 },
        publishedAt: "2026-08-20T00:26:59Z",
        latest: true,
        paused: false,
        stage: "copilot",
        activity: {
          runs: 19,
          finished: 17,
          waiting: 0,
          costMicros: 0,
          lastPhase: "finished",
          lastRunAt: "2026-08-20T00:28:33Z",
        },
      },
      instructions: "The definition is not the landing view.",
      source: "console",
      steps: [],
      versions: [{ versionId: "vb6148c24", latest: true, publishedAt: "2026-08-20T00:26:59Z" }],
    };
    detail.trust = {
      versionId: "vb6148c24",
      status: "ready",
      recommendation: "autonomous",
      summary: "ready",
      window: {
        from: "2026-08-01T00:00:00.000Z",
        until: "2026-08-31T00:00:00.000Z",
      },
      evidence: [
        {
          id: "simulation",
          status: "good",
          code: "simulation_ready",
          values: { cases: 1, held: 1 },
        },
      ],
    };
  });

  it("opens on runs and leaves the definition behind a tab", async () => {
    showDetail();

    expect(screen.getByRole("tab", { name: /Runs 19/ })).toHaveAttribute(
      "data-state",
      "active",
    );
    for (const tab of screen.getAllByRole("tab")) {
      expect(tab).toHaveClass("flex-none");
      expect(tab.className).toContain(
        "group-data-[variant=line]/tabs-list:rounded-none",
      );
      expect(tab.className).toContain(
        "group-data-[orientation=horizontal]/tabs:after:bottom-0",
      );
      expect(tab.className).not.toContain("after:bottom-[-5px]");
    }
    expect(screen.getByLabelText("runs panel")).toHaveAttribute(
      "data-header",
      "no",
    );
    expect(screen.queryByText("The definition is not the landing view.")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: "Definition" }));

    expect(
      screen.getByText("The definition is not the landing view."),
    ).toBeInTheDocument();
    expect(screen.getByTestId("capability-rail")).toHaveAttribute(
      "data-compact",
      "yes",
    );
    expect(screen.getByTestId("versions-rail")).toHaveAttribute(
      "data-compact",
      "yes",
    );
  });
});
