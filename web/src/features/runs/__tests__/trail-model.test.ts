import { beforeEach, describe, expect, it } from "vitest";
import { buildTrail, type TrailGroup } from "@/features/runs/trail-model";
import type { Step, StepKind } from "@/lib/api/client";

let seq = 0;
function step(kind: StepKind, payload: Record<string, unknown> = {}): Step {
  seq += 1;
  return { seq, kind, at: "2026-08-11T14:24:59Z", hash: `hash${seq}`, payload };
}

const ALLOW = { verdict: "allow", tool: "crm.lookup" };
const ESCALATE = { verdict: "require_approval", tool: "kb.search" };

function routineCycle(): Step[] {
  return [
    step("planned"),
    step("gate_decided", ALLOW),
    step("budget_reserved"),
    step("tool_called", { tool: "crm.lookup" }),
    step("tool_returned", { tool: "crm.lookup" }),
    step("budget_reconciled"),
  ];
}

function entriesOf(groups: TrailGroup[]) {
  return groups.flatMap((group) => group.entries);
}

beforeEach(() => {
  seq = 0;
});

describe("the trail's phases", () => {
  it("separates what started the run from what it then did", () => {
    const groups = buildTrail([step("run_started"), ...routineCycle()], {
      filter: "all",
    });

    expect(groups[0]?.phase).toBe("input");
    expect(groups[0]?.entries).toHaveLength(2); // run_started, planned
    expect(groups[1]?.phase).toBe("execution");
  });

  it("opens a human phase at the escalation, not at the approval itself", () => {
    // The decision the reader is looking for is the gate's: it is what
    // explains why a human was called at all.
    const groups = buildTrail(
      [
        step("run_started"),
        step("planned"),
        step("gate_decided", ESCALATE),
        step("approval_requested"),
      ],
      { filter: "all" },
    );

    const human = groups.find((g) => g.phase === "human");
    expect(human?.entries[0]).toMatchObject({ step: { kind: "gate_decided" } });
  });

  it("returns to execution once a human has decided", () => {
    const groups = buildTrail(
      [
        step("run_started"),
        step("gate_decided", ESCALATE),
        step("approval_requested"),
        step("approval_decided", { approved: true }),
        ...routineCycle(),
      ],
      { filter: "all" },
    );

    expect(groups.map((g) => g.phase)).toEqual(["input", "human", "execution"]);
  });
});

describe("folding routine steps", () => {
  it("folds a long run of read-only allowed steps into one entry", () => {
    // An eighteen-event run must not read as eighteen identical blocks: the
    // eye should land on the decisions, not on the bookkeeping. An allowed
    // read is bookkeeping, gate decision included.
    const groups = buildTrail(
      [step("run_started"), ...routineCycle(), ...routineCycle()],
      {
        filter: "all",
      },
    );

    const folds = entriesOf(groups).filter((e) => e.kind === "fold");
    expect(folds).toHaveLength(1);
    expect(folds[0]?.kind === "fold" && folds[0].steps.length).toBe(11);
  });

  it("never folds an escalation or the approval it asked for", () => {
    // An allowed read folds; the decision that stopped the run does not,
    // whatever the surrounding bookkeeping looks like.
    const escalation = step("gate_decided", ESCALATE);
    const request = step("approval_requested");
    const groups = buildTrail(
      [step("run_started"), ...routineCycle(), escalation, request],
      { filter: "all" },
    );

    const folded = entriesOf(groups).flatMap((e) =>
      e.kind === "fold" ? e.steps : [],
    );
    expect(folded.map((s) => s.seq)).not.toContain(escalation.seq);
    expect(folded.map((s) => s.seq)).not.toContain(request.seq);
  });

  it("leaves a short run of steps alone rather than hiding two things behind a fold", () => {
    const groups = buildTrail(
      [step("run_started"), step("planned"), step("gate_decided", ALLOW)],
      {
        filter: "all",
      },
    );

    expect(entriesOf(groups).every((e) => e.kind === "step")).toBe(true);
  });
});

describe("the trail's filters", () => {
  it("narrows to the calls the agent made", () => {
    const groups = buildTrail([step("run_started"), ...routineCycle()], {
      filter: "tools",
    });

    const kinds = entriesOf(groups).flatMap((e) =>
      e.kind === "step" ? [e.step.kind] : [],
    );
    expect(new Set(kinds)).toEqual(new Set(["tool_called", "tool_returned"]));
  });

  it("narrows to what the gate and the humans decided", () => {
    const groups = buildTrail(
      [
        step("run_started"),
        ...routineCycle(),
        step("gate_decided", ESCALATE),
        step("approval_requested"),
      ],
      { filter: "policy" },
    );

    const kinds = entriesOf(groups).flatMap((e) =>
      e.kind === "step" ? [e.step.kind] : [],
    );
    expect(new Set(kinds)).toEqual(
      new Set(["gate_decided", "approval_requested"]),
    );
  });

  it("never folds inside a narrowed trail, where every remaining step is the point", () => {
    const groups = buildTrail(
      [step("run_started"), ...routineCycle(), ...routineCycle()],
      {
        filter: "tools",
      },
    );

    expect(entriesOf(groups).every((e) => e.kind === "step")).toBe(true);
  });
});
