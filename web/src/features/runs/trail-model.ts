import type { Step, StepKind } from "@/lib/api/client";
import { verdictOf } from "@/features/runs/step-verb";

/**
 * The trail, arranged so a person can read it.
 *
 * A run is a sequence of steps, but a list of eighteen of them reads as
 * eighteen identical blocks, and the two that matter — the escalation and the
 * human decision — carry no more weight than a budget reservation. This
 * shapes the same steps into phases and folds the bookkeeping away without
 * ever hiding anything: a fold names how many steps it holds, and opens.
 *
 * Pure on purpose. The ordering rules are the product's, not the layout's.
 */

/** The four things a stretch of a run can be about. */
export type TrailPhase = "input" | "execution" | "human" | "end";

/** What the reader asked to see. */
export type TrailFilter = "all" | "tools" | "policy";

export type TrailEntry =
  | { kind: "step"; step: Step }
  | { kind: "fold"; steps: Step[] };

export interface TrailGroup {
  phase: TrailPhase;
  /** When the phase opened. */
  at: string;
  entries: TrailEntry[];
}

/** Steps a fold may swallow: allowed, read-only, no human involved. */
const ROUTINE: ReadonlySet<StepKind> = new Set<StepKind>([
  "planned",
  "budget_reserved",
  "tool_called",
  "tool_returned",
  "budget_reconciled",
]);

/**
 * Below this many consecutive routine steps, folding costs the reader more
 * than it saves: a fold hiding two steps is one more thing to open.
 */
const FOLD_FROM = 4;

const KEPT: Record<TrailFilter, (step: Step) => boolean> = {
  all: () => true,
  tools: (step) => step.kind === "tool_called" || step.kind === "tool_returned",
  policy: (step) =>
    step.kind === "gate_decided" ||
    step.kind === "approval_requested" ||
    step.kind === "approval_decided",
};

/**
 * The steps a filter leaves standing.
 *
 * Shared with the diagram so the two views of one run cannot disagree: a
 * reader who narrows to policy and sees eighteen nodes beside three rows would
 * be right to distrust both.
 */
export function keptSteps(steps: Step[], filter: TrailFilter): Step[] {
  return steps.filter(KEPT[filter]);
}

export function buildTrail(steps: Step[], opts: { filter: TrailFilter }): TrailGroup[] {
  const groups: TrailGroup[] = [];

  let decided = false;
  for (const step of keptSteps(steps, opts.filter)) {
    const phase = phaseOf(step, decided);
    if (phase !== "input") decided = true;

    const current = groups.at(-1);
    if (current?.phase === phase) {
      current.entries.push({ kind: "step", step });
      continue;
    }
    groups.push({ phase, at: step.at, entries: [{ kind: "step", step }] });
  }

  // A narrowed trail folds nothing: the reader has already said which steps
  // are the point, and folding them would answer a question they did not ask.
  if (opts.filter !== "all") return groups;
  return groups.map((group) => ({ ...group, entries: fold(group.entries) }));
}

/**
 * The phase a step belongs to.
 *
 * `input` is what started the run and the first thing the agent thought about
 * it — everything before the platform first decided anything. `human` opens at
 * the gate decision that escalated rather than at the approval request,
 * because the escalation is what explains why a human was called at all.
 */
function phaseOf(step: Step, decided: boolean): TrailPhase {
  switch (step.kind) {
    case "run_started":
      return "input";
    case "planned":
      return decided ? "execution" : "input";
    case "approval_requested":
    case "approval_decided":
      return "human";
    case "gate_decided":
      return verdictOf(step) === "require_approval" ? "human" : "execution";
    case "parked":
    case "failed":
    case "run_finished":
      return "end";
    default:
      return "execution";
  }
}

/** Collapses each long stretch of routine steps into a single entry. */
function fold(entries: TrailEntry[]): TrailEntry[] {
  const out: TrailEntry[] = [];
  let run: Step[] = [];

  const flush = () => {
    if (run.length >= FOLD_FROM) out.push({ kind: "fold", steps: run });
    else out.push(...run.map((step): TrailEntry => ({ kind: "step", step })));
    run = [];
  };

  for (const entry of entries) {
    if (entry.kind === "step" && isRoutine(entry.step)) {
      run.push(entry.step);
      continue;
    }
    flush();
    out.push(entry);
  }
  flush();
  return out;
}

/**
 * A step is routine when nothing about it needed a person.
 *
 * A gate decision that allowed a read is bookkeeping and folds with the rest;
 * a constraint, an escalation or a block is the reason the screen exists and
 * never does. A tool that came back carrying untrusted data is not routine
 * either, even though its kind is: that label is why the next call was
 * escalated, and burying it hides the cause one row above its effect.
 */
function isRoutine(step: Step): boolean {
  if (step.kind === "gate_decided") return verdictOf(step) === "allow";
  if (!ROUTINE.has(step.kind)) return false;
  return !(step.labels ?? []).includes("untrusted");
}
