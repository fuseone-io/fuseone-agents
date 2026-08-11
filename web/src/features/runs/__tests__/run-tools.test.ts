import { describe, expect, it } from "vitest";
import { toolsOf } from "@/features/runs/run-tools";
import type { Step, StepKind } from "@/lib/api/client";

let seq = 0;
function step(kind: StepKind, payload: Record<string, unknown>): Step {
  seq += 1;
  return { seq, kind, at: "2026-08-11T14:24:59Z", hash: `h${seq}`, payload };
}

// The ledger encodes effect as the domain's integer. Comparing it to the
// string "read" made every call look like a write, so a run that only read
// reported that it had changed a real system.
const READ = 1;
const WRITE = 2;

describe("what a run touched", () => {
  it("reports no write when every call was a read", () => {
    const tools = toolsOf([
      step("tool_called", { tool: "crm.lookup", effect: READ }),
      step("tool_returned", { tool: "crm.lookup" }),
    ]);

    expect(tools.some((t) => t.wrote)).toBe(false);
  });

  it("reports a write as soon as one call had an effect on the world", () => {
    const tools = toolsOf([
      step("tool_called", { tool: "crm.lookup", effect: READ }),
      step("tool_called", { tool: "crm.note", effect: WRITE }),
    ]);

    expect(tools.find((t) => t.name === "crm.note")?.wrote).toBe(true);
    expect(tools.find((t) => t.name === "crm.lookup")?.wrote).toBe(false);
  });

  it("marks the tool the gate stopped, which is the one the reader is looking for", () => {
    const tools = toolsOf([
      step("gate_decided", { tool: "kb.search", verdict: 3 }),
      step("approval_requested", { tool: "kb.search" }),
    ]);

    expect(tools[0]?.escalated).toBe(true);
  });
});
