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
  run_started: { verb: "iniciou", tone: "neutral" },
  planned: { verb: "propôs", tone: "neutral" },
  gate_decided: { verb: "decidiu", tone: "neutral" },
  budget_reserved: { verb: "reservou", tone: "neutral" },
  tool_called: { verb: "chamou", tone: "neutral" },
  tool_returned: { verb: "respondeu", tone: "neutral" },
  budget_reconciled: { verb: "conciliou", tone: "neutral" },
  approval_requested: { verb: "pediu aprovação", tone: "warn" },
  approval_decided: { verb: "decidiu", tone: "good" },
  compensated: { verb: "reverteu", tone: "warn" },
  failed: { verb: "falhou", tone: "bad" },
  parked: { verb: "estacionou", tone: "warn" },
  run_finished: { verb: "concluiu", tone: "good" },
};

const VERDICTS: Record<string, { verb: string; tone: Tone }> = {
  allow: { verb: "permitiu", tone: "good" },
  constrain: { verb: "restringiu", tone: "warn" },
  require_approval: { verb: "exigiu aprovação", tone: "warn" },
  block: { verb: "bloqueou", tone: "bad" },
};

// The wire encodes verdict as the domain's integer; map it back for display.
const VERDICT_BY_CODE = ["unknown", "allow", "constrain", "require_approval", "block"];

export function verbOf(step: Step): { verb: string; tone: Tone } {
  if (step.kind === "gate_decided") {
    const payload = (step.payload ?? {}) as Record<string, unknown>;
    const raw = payload.verdict;
    const name = typeof raw === "number" ? VERDICT_BY_CODE[raw] : raw;
    if (typeof name === "string" && VERDICTS[name]) return VERDICTS[name];
  }
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
