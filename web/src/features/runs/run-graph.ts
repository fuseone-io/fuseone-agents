import { effectOf, verdictOf } from "@/features/runs/step-verb";
import type { TileTone } from "@/features/runs/trail-icon";
import type { Step, StepKind } from "@/lib/api/client";

/**
 * A run, shaped as a graph.
 *
 * The specification has no graph — instructions, a tool pack, ceilings and
 * triggers, with the loop belonging to the interpreter. What does have one is
 * the execution, and the handoff's own prototype settles which was meant: every
 * node in it carries a latency and a health, and a specification does not have
 * 12ms. So this draws the ledger, and the picture is of one run.
 *
 * Pure. The layout is a separate concern and the render model is never stored
 * (PRD FU-18): the versioned specification and the ledger are the truth, and
 * nodes are a projection that can be thrown away.
 */

/** Eight kinds, and no `branch`: nothing in the ledger records one, because
 *  the loop is the interpreter's rather than something somebody authored. */
export type FlowKind =
  | "trigger"
  | "agent"
  | "policy"
  | "tool"
  | "action"
  | "human"
  | "seal"
  | "fault";

export interface FlowNode {
  id: string;
  kind: FlowKind;
  title: string;
  detail?: string;
  /** Milliseconds. Absent when the pair never closed — rendering zero would
   *  say a call still in flight answered instantly. */
  latencyMs?: number;
  tone: TileTone;
  /** The step it anchors to, so the picture can point back at the record. */
  seq: number;
}

export interface FlowEdge {
  from: string;
  to: string;
}

/** Steps that are accounting rather than events in the story. */
const BOOKKEEPING: ReadonlySet<StepKind> = new Set<StepKind>([
  "budget_reserved",
  "budget_reconciled",
]);

/** The second half of a pair, folded into the node its opener made. */
const CLOSERS: ReadonlySet<StepKind> = new Set<StepKind>([
  "tool_returned",
  "approval_decided",
]);

export function buildGraph(steps: Step[]): {
  nodes: FlowNode[];
  edges: FlowEdge[];
} {
  const nodes: FlowNode[] = [];

  steps.forEach((step, i) => {
    if (BOOKKEEPING.has(step.kind) || CLOSERS.has(step.kind)) return;

    const closer = closerFor(step, steps.slice(i + 1));
    nodes.push({
      id: `s${step.seq}`,
      seq: step.seq,
      ...describe(step),
      latencyMs: elapsed(step, closer ?? previousOf(steps, i)),
    });
  });

  const edges: FlowEdge[] = [];
  for (let i = 1; i < nodes.length; i++) {
    edges.push({ from: nodes[i - 1]!.id, to: nodes[i]!.id });
  }
  return { nodes, edges };
}

/** The step that closes this one, if it arrived. */
function closerFor(step: Step, rest: Step[]): Step | undefined {
  const wanted: Partial<Record<StepKind, StepKind>> = {
    tool_called: "tool_returned",
    approval_requested: "approval_decided",
  };
  const kind = wanted[step.kind];
  if (!kind) return undefined;
  // The first one after it: a run can call the same tool twice, and matching
  // by name would pair the first call with the second answer.
  return rest.find((next) => next.kind === kind);
}

/** How long the node took, or how long since the last thing happened. */
function elapsed(step: Step, other: Step | undefined): number | undefined {
  if (!other) return undefined;
  const ms = Date.parse(other.at) - Date.parse(step.at);
  return Number.isFinite(ms) ? Math.abs(ms) : undefined;
}

function previousOf(steps: Step[], i: number): Step | undefined {
  return i === 0 ? undefined : steps[i - 1];
}

function describe(
  step: Step,
): Pick<FlowNode, "kind" | "title" | "detail" | "tone"> {
  const payload = (step.payload ?? {}) as Record<string, unknown>;
  const text = (key: string) =>
    typeof payload[key] === "string" ? payload[key] : undefined;

  switch (step.kind) {
    case "run_started":
      return {
        kind: "trigger",
        title: "runs.nodeStarted",
        detail: text("trigger"),
        tone: "neutral",
      };
    case "planned":
      return {
        kind: "agent",
        title: "runs.nodeProposed",
        detail: text("tool"),
        tone: "neutral",
      };
    case "gate_decided":
      return gate(step, text("rule"));
    case "tool_called":
      return {
        // A read and a write must not look alike: which node touched a real
        // system is the one thing this picture exists to show.
        kind: effectOf(step) === "read" ? "tool" : "action",
        title: text("tool") ?? "ferramenta",
        detail: effectOf(step),
        tone: "neutral",
      };
    case "approval_requested":
      return {
        kind: "human",
        title: "runs.nodeHuman",
        detail: text("rule"),
        tone: "escalate",
      };
    case "compensated":
      return { kind: "fault", title: "runs.nodeCompensated", tone: "escalate" };
    case "failed":
      return {
        kind: "fault",
        title: "Falhou",
        detail: text("reason"),
        tone: "block",
      };
    case "parked":
      return {
        kind: "fault",
        title: "runs.phaseParked",
        detail: text("reason"),
        tone: "escalate",
      };
    default:
      return { kind: "seal", title: "runs.nodeSealed", tone: "allow" };
  }
}

const GATE_TONE: Record<string, TileTone> = {
  allow: "allow",
  constrain: "escalate",
  require_approval: "escalate",
  block: "block",
  duplicate: "neutral",
};

const GATE_TITLE: Record<string, string> = {
  allow: "runs.nodeAllowed",
  constrain: "runs.nodeConstrained",
  require_approval: "runs.nodeEscalated",
  block: "runs.nodeBlocked",
  duplicate: "runs.nodeSkipped",
};

/** The rule, never only the verdict: "blocked by policy" tells a reader
 *  nothing about what to change. */
function gate(
  step: Step,
  rule?: string,
): Pick<FlowNode, "kind" | "title" | "detail" | "tone"> {
  const verdict = verdictOf(step) ?? "allow";
  return {
    kind: "policy",
    title: GATE_TITLE[verdict] ?? "runs.nodeDecided",
    detail: rule,
    tone: GATE_TONE[verdict] ?? "neutral",
  };
}
