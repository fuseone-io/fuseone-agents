import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RunSpendPanel } from "@/features/runs/run-spend-panel";
import type { Run, Step } from "@/lib/api/client";

const run = {
  runId: "run-1", agentId: "a", versionId: "v", phase: "finished", seq: 9,
  startedAt: "2026-08-23T00:00:00Z", scope: { company: "acme", area: "cx" },
  cost: { micros: 900, inputTokens: 100 },
} as unknown as Run;

function planned(prompt: Record<string, unknown>): Step {
  return { seq: 1, kind: "planned", at: "x", hash: "h", payload: { prompt } } as Step;
}

describe("the spend panel", () => {
  it("names the tool compaction saved the most on", () => {
    // The number alone says compaction is working. The tool says where the
    // next one is, which is the decision this panel exists to support.
    render(
      <RunSpendPanel
        run={run}
        steps={[
          planned({
            unit: "content_bytes",
            tool_results: 10_000,
            tool_results_elided: 90_000,
            tool_results_elided_by_tool: { "grafana.query_loki_logs": 90_000 },
            total: 10_000,
          }),
        ]}
      />,
    );

    expect(screen.getByText("grafana.query_loki_logs")).toBeInTheDocument();
  });
});
