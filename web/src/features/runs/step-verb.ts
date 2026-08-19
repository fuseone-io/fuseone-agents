import type { Step, StepKind } from "@/lib/api/client";

export type Tone = "neutral" | "good" | "warn" | "bad";

/**
 * What a step did, as one word.
 *
 * The trail reads as a sequence of acts rather than a table of records, which
 * is what lets an approver and an auditor read the same screen at different
 * depths (PRD AU-09). The word carries the meaning; the colour only repeats it.
 */
const VERBS: Record<StepKind, { verb: string; tone: Tone }> = {
  run_started: { verb: "runs.verbStarted", tone: "neutral" },
  planned: { verb: "runs.verbProposed", tone: "neutral" },
  gate_decided: { verb: "runs.verbDecided", tone: "neutral" },
  budget_reserved: { verb: "runs.verbReserved", tone: "neutral" },
  tool_called: { verb: "runs.verbCalled", tone: "neutral" },
  tool_returned: { verb: "runs.verbReturned", tone: "neutral" },
  budget_reconciled: { verb: "runs.verbReconciled", tone: "neutral" },
  approval_requested: { verb: "runs.verbAskedApproval", tone: "warn" },
  approval_decided: { verb: "runs.verbDecided", tone: "good" },
  resumed: { verb: "runs.verbResumed", tone: "good" },
  abandoned: { verb: "runs.verbAbandoned", tone: "bad" },
  compensated: { verb: "runs.verbCompensated", tone: "warn" },
  failed: { verb: "runs.verbFailed", tone: "bad" },
  parked: { verb: "runs.verbParked", tone: "warn" },
  run_finished: { verb: "runs.verbFinished", tone: "good" },
};

const VERDICTS: Record<string, { verb: string; tone: Tone }> = {
  allow: { verb: "runs.verbAllowed", tone: "good" },
  constrain: { verb: "runs.verbConstrained", tone: "warn" },
  require_approval: { verb: "runs.verbRequiredApproval", tone: "warn" },
  block: { verb: "runs.verbBlocked", tone: "bad" },
};

// The wire encodes verdict and effect as the domain's integers; map them back
// for display. Switching on the number would break silently the day a value is
// inserted in the middle of either list.
const VERDICT_BY_CODE = [
  "unknown",
  "allow",
  "constrain",
  "require_approval",
  "block",
];
const EFFECT_BY_CODE = ["unknown", "read", "write", "destructive", "financial"];

/** A step's effect, by name, whatever form the payload carries it in. */
export function effectOf(step: Step): string | undefined {
  const raw = (step.payload as Record<string, unknown> | undefined)?.effect;
  const name = typeof raw === "number" ? EFFECT_BY_CODE[raw] : raw;
  return typeof name === "string" ? name : undefined;
}

/**
 * The gate's verdict, by name.
 *
 * The wire carries the domain's integer; a screen that switched on the number
 * would break silently the day a verdict is inserted in the middle.
 */
export function verdictOf(step: Step): string | undefined {
  if (step.kind !== "gate_decided") return undefined;
  const raw = (step.payload as Record<string, unknown> | undefined)?.verdict;
  const name = typeof raw === "number" ? VERDICT_BY_CODE[raw] : raw;
  return typeof name === "string" ? name : undefined;
}

export function verbOf(step: Step): { verb: string; tone: Tone } {
  const verdict = verdictOf(step);
  if (verdict && VERDICTS[verdict]) return VERDICTS[verdict];
  return VERBS[step.kind];
}

export const TONE_TEXT: Record<Tone, string> = {
  neutral: "text-muted-foreground",
  good: "text-success",
  warn: "text-warning",
  bad: "text-danger",
};

export const TONE_DOT: Record<Tone, string> = {
  neutral: "bg-border-strong",
  good: "bg-success",
  warn: "bg-warning",
  bad: "bg-danger",
};
