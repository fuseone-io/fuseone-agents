import { describe, expect, it } from "vitest";
import { buildGraph } from "@/features/runs/run-graph";
import type { Step, StepKind } from "@/lib/api/client";

let seq = 0;
const step = (kind: StepKind, at: string, payload: Record<string, unknown> = {}): Step =>
  ({ seq: ++seq, kind, at, hash: "", payload }) as Step;

describe("a run drawn as a graph", () => {
  it("pairs a call with its answer into one node carrying how long it took", () => {
    const { nodes } = buildGraph([
      step("tool_called", "2026-08-12T10:00:00Z", { tool: "crm.lookup", effect: 1 }),
      step("tool_returned", "2026-08-12T10:00:00.180Z", { tool: "crm.lookup" }),
    ]);

    expect(nodes).toHaveLength(1);
    expect(nodes[0]).toMatchObject({ kind: "tool", title: "crm.lookup", latencyMs: 180 });
  });

  it("draws a call that changed something as an action, not as a lookup", () => {
    // The distinction is the product's whole point: two nodes that look alike
    // would hide which one touched a real system.
    const { nodes } = buildGraph([
      step("tool_called", "2026-08-12T10:00:00Z", { tool: "crm.reply", effect: 2 }),
      step("tool_returned", "2026-08-12T10:00:00.220Z", {}),
    ]);

    expect(nodes[0]?.kind).toBe("action");
  });

  it("pairs an escalation with its answer, carrying the wait", () => {
    const { nodes } = buildGraph([
      step("approval_requested", "2026-08-12T10:00:00Z", {}),
      step("approval_decided", "2026-08-12T10:05:00Z", { approved: true }),
    ]);

    expect(nodes).toHaveLength(1);
    expect(nodes[0]).toMatchObject({ kind: "human", latencyMs: 300_000 });
  });

  it("leaves a call still in flight as a node with no elapsed time", () => {
    // Rendering zero would say it answered instantly, which is the opposite of
    // what is happening.
    const { nodes } = buildGraph([
      step("tool_called", "2026-08-12T10:00:00Z", { tool: "crm.lookup", effect: 1 }),
    ]);

    expect(nodes[0]?.latencyMs).toBeUndefined();
  });

  it("keeps bookkeeping out of the picture", () => {
    const { nodes } = buildGraph([
      step("run_started", "2026-08-12T10:00:00Z", { trigger: "webhook" }),
      step("budget_reserved", "2026-08-12T10:00:01Z", {}),
      step("budget_reconciled", "2026-08-12T10:00:02Z", {}),
      step("run_finished", "2026-08-12T10:00:03Z", {}),
    ]);

    expect(nodes.map((n) => n.kind)).toEqual(["trigger", "seal"]);
  });

  it("chains the nodes in the order they happened", () => {
    const { nodes, edges } = buildGraph([
      step("run_started", "2026-08-12T10:00:00Z", {}),
      step("planned", "2026-08-12T10:00:01Z", {}),
      step("run_finished", "2026-08-12T10:00:02Z", {}),
    ]);

    expect(edges).toHaveLength(nodes.length - 1);
    expect(edges[0]).toMatchObject({ from: nodes[0]?.id, to: nodes[1]?.id });
  });

  it("names the rule a gate decision applied, never just its verdict", () => {
    const { nodes } = buildGraph([
      step("gate_decided", "2026-08-12T10:00:00Z", { verdict: 4, rule: "POL-114" }),
    ]);

    expect(nodes[0]).toMatchObject({ kind: "policy", detail: "POL-114", tone: "block" });
  });
});
