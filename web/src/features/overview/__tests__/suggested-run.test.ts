import { describe, expect, it } from "vitest";
import { suggestedRun } from "@/features/overview/suggested-run";
import type { Run } from "@/lib/api/client";

function run(over: Partial<Run>): Run {
  return {
    runId: "run-1",
    scope: { company: "acme", area: "cx" },
    agentId: "triage",
    versionId: "v1",
    phase: "finished",
    seq: 10,
    startedAt: "2026-08-11T12:00:00Z",
    cost: { micros: 0 },
    ...over,
  };
}

describe("which run the trace opens on", () => {
  it("prefers one waiting for a person over one that merely finished", () => {
    // The panel exists so somebody can act. Opening on the newest run would
    // show a finished one while an approval sat below it unread.
    const chosen = suggestedRun([
      run({ runId: "run-done", phase: "finished" }),
      run({ runId: "run-waiting", phase: "awaiting_approval" }),
    ]);

    expect(chosen).toBe("run-waiting");
  });

  it("falls back to the most recent when nothing is waiting", () => {
    expect(
      suggestedRun([run({ runId: "run-new" }), run({ runId: "run-old" })]),
    ).toBe("run-new");
  });

  it("opens on nothing when nothing ran", () => {
    // An empty panel taking a third of the width to say so is worse than no
    // panel.
    expect(suggestedRun([])).toBeUndefined();
  });
});
