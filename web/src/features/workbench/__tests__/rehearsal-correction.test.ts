import { describe, expect, it } from "vitest";
import { rehearsalCase } from "@/features/workbench/rehearsal-correction";
import type { Run, Step } from "@/lib/api/client";

/*
A rehearsal is corrected the way a simulated case is (NT-018 phase 5b): what the author is
offered is read from what the rehearsal did, folded the same way a simulation report folds
a case. Each decision the Gate made is an act, at the step planned before it, and the call
that followed it is what says the act got as far as the tool layer.
*/

const run = (phase: Run["phase"]): Run => ({
  runId: "run-reh", scope: { company: "acme", area: "cx" }, agentId: "triagem", versionId: "v2abc",
  phase, seq: 9, startedAt: "2026-11-20T12:00:00Z", cost: { micros: 0 },
});
const step = (seq: number, kind: Step["kind"], payload: Record<string, unknown> = {}): Step => ({
  seq, kind, at: "2026-11-20T12:00:00Z", hash: `h${seq}`, payload,
});

describe("a rehearsal read as a case to correct", () => {
  it("pairs each call with the decision before it, at the step planned for it", () => {
    const trail = [
      step(1, "run_started"),
      step(2, "planned", { node: "lookup" }),
      step(3, "gate_decided", { tool: "crm.lookup", effect: "read" }),
      step(4, "tool_called", { tool: "crm.lookup" }),
      step(5, "planned", { node: "reply" }),
      step(6, "gate_decided", { tool: "crm.reply", effect: "write" }),
      step(7, "run_finished"),
    ];
    expect(rehearsalCase(run("finished"), trail)).toEqual({
      runId: "run-reh",
      settled: "finished",
      acted: [
        { step: "lookup", tool: "crm.lookup", reached: true },
        { step: "reply", tool: "crm.reply", reached: false },
      ],
    });
  });

  it("names no settling for a rehearsal that ended without finishing", () => {
    expect(rehearsalCase(run("failed"), [step(1, "run_started")]).settled).toBeUndefined();
  });

  it("offers nothing about a decision that names no tool", () => {
    expect(rehearsalCase(run("finished"), [step(1, "gate_decided"), step(2, "tool_called")]).acted).toEqual([]);
  });
});
