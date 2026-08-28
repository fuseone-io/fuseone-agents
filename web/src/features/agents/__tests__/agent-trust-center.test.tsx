import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { AgentTrustCenter } from "@/features/agents/agent-trust-center";
import { setLocale } from "@/i18n";
import type { Agent } from "@/lib/api/client";

describe("AgentTrustCenter", () => {
  it("renders the server's stable trust evidence instead of recomputing it", () => {
    setLocale("en-US");

    render(
      <MemoryRouter>
        <AgentTrustCenter
          agent={agent()}
          trust={{
            versionId: "v2",
            status: "needs_review",
            recommendation: "review",
            summary: "review",
            previousVersionId: "v1",
            window: {
              from: "2026-08-01T00:00:00.000Z",
              until: "2026-08-31T00:00:00.000Z",
            },
            evidence: [
              {
                id: "cost",
                status: "bad",
                code: "cost_increased",
                values: {
                  currentAverageMicros: 900_000,
                  previousAverageMicros: 100_000,
                },
              },
              {
                id: "decisions",
                status: "unknown",
                code: "decisions_waiting",
                values: { waiting: 1 },
              },
            ],
          }}
        />
      </MemoryRouter>,
    );

    expect(screen.getByText("needs review")).toBeInTheDocument();
    expect(screen.getByText(/Run, cost, Gate and human-decision evidence use/)).toBeInTheDocument();
    expect(screen.getByText("Cost comparison")).toBeInTheDocument();
    expect(screen.getByText(/Average cost moved from R\$0\.10 to R\$0\.90/)).toBeInTheDocument();
    expect(screen.getByText("Human decisions")).toBeInTheDocument();
    expect(screen.getByText("1 run(s) are waiting for a person.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Cost comparison/ })).toHaveAttribute(
      "href",
      "/cost",
    );
  });

  it("explains how missing comparison evidence appears and when no action exists", () => {
    setLocale("en-US");

    render(
      <MemoryRouter>
        <AgentTrustCenter
          agent={agent()}
          trust={{
            versionId: "v2",
            previousVersionId: "v1",
            status: "needs_evidence",
            recommendation: "collect",
            summary: "evidence",
            window: {
              from: "2026-08-01T00:00:00.000Z",
              until: "2026-08-31T00:00:00.000Z",
            },
            evidence: [
              {
                id: "version",
                status: "missing",
                code: "version_missing_baseline",
                values: {},
              },
              {
                id: "cost",
                status: "unknown",
                code: "cost_missing",
                values: {},
              },
              {
                id: "decisions",
                status: "missing",
                code: "decisions_missing",
                values: {},
              },
            ],
          }}
        />
      </MemoryRouter>,
    );

    expect(
      screen.getByText(/Run the saved situations on this version/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/priced runs from this version and the previous version/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/There is nothing to trigger manually/),
    ).toBeInTheDocument();
    expect(screen.getByText("not observed")).toBeInTheDocument();
  });
});

function agent(): Agent {
  return {
    agentId: "triage",
    versionId: "v2",
    scope: { company: "acme", area: "platform" },
    name: "Triage",
    provider: "anthropic",
    model: "claude-sonnet-5",
    tools: [],
    budget: { steps: 20 },
    publishedAt: "2026-08-24T10:00:00.000Z",
    latest: true,
    paused: false,
    stage: "copilot",
  };
}
