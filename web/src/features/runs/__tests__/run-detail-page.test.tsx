import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { RunDetailPage } from "@/features/runs/run-detail-page";
import { setLocale } from "@/i18n";
import type { Run, Step } from "@/lib/api/client";

const detail = vi.hoisted(() => ({
  run: undefined as Run | undefined,
  steps: [] as Step[],
}));

vi.mock("@/features/runs/api", () => ({
  useRun: () => ({
    data: detail.run,
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  }),
  useRunSteps: () => ({
    items: detail.steps,
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  }),
}));

vi.mock("@/features/runs/run-identity", () => ({
  RunIdentity: () => <header data-testid="run-identity" />,
}));

vi.mock("@/features/runs/run-failure-notice", () => ({
  RunFailureNotice: () => null,
}));

vi.mock("@/features/runs/pending-decision", () => ({
  PendingDecision: () => <section data-testid="pending-decision" />,
}));

vi.mock("@/features/runs/run-kpis", () => ({
  RunKpis: () => <section data-testid="run-kpis" />,
}));

vi.mock("@/features/runs/trail-panel", () => ({
  TrailPanel: () => <section aria-label="trail panel" />,
}));

vi.mock("@/features/runs/run-side-rail", () => ({
  RunSideRail: () => <aside aria-label="run side rail" />,
}));

vi.mock("@/features/runs/run-spend-panel", () => ({
  RunSpendPanel: () => <section aria-label="cost panel" />,
}));

function showRun() {
  return render(
    <MemoryRouter initialEntries={["/runs/run-1"]}>
      <Routes>
        <Route path="/runs/:runId" element={<RunDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("the run overview", () => {
  beforeEach(() => {
    setLocale("en-US");
    detail.steps = [];
    detail.run = {
      runId: "run-1",
      scope: { company: "acme", area: "platform" },
      agentId: "troubleshooting-devops",
      versionId: "v1",
      phase: "finished",
      seq: 8,
      startedAt: "2026-08-28T12:00:00Z",
      endedAt: "2026-08-28T12:00:06Z",
      cost: { micros: 9_900, inputTokens: 10_779 },
    };
  });

  it("opens on the trail and keeps cost behind a matching tab", async () => {
    showRun();

    const trail = screen.getByRole("tab", { name: "Trail" });
    const cost = screen.getByRole("tab", { name: "Cost" });

    expect(trail).toHaveAttribute("data-state", "active");
    expect(trail.querySelector("svg")).not.toBeNull();
    expect(cost.querySelector("svg")).not.toBeNull();
    for (const tab of [trail, cost]) {
      expect(tab).toHaveClass("flex-none");
      expect(tab.className).toContain(
        "group-data-[variant=line]/tabs-list:rounded-none",
      );
      expect(tab.className).toContain(
        "group-data-[orientation=horizontal]/tabs:after:bottom-0",
      );
    }

    expect(screen.getByTestId("run-identity")).toBeInTheDocument();
    expect(screen.getByTestId("run-kpis")).toBeInTheDocument();
    expect(screen.getByLabelText("trail panel")).toBeInTheDocument();
    expect(screen.getByLabelText("run side rail")).toBeInTheDocument();
    expect(screen.queryByLabelText("cost panel")).not.toBeInTheDocument();

    await userEvent.click(cost);

    expect(cost).toHaveAttribute("data-state", "active");
    expect(screen.getByTestId("run-identity")).toBeInTheDocument();
    expect(screen.getByTestId("run-kpis")).toBeInTheDocument();
    expect(screen.getByLabelText("cost panel")).toBeInTheDocument();
    expect(screen.queryByLabelText("trail panel")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("run side rail")).not.toBeInTheDocument();
  });
});
