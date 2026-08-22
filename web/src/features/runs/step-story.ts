import { effectOf, verdictOf } from "@/features/runs/step-verb";
import { explainRule } from "@/lib/gate-rules";
import {
  formatCost,
  formatDurationMs,
  formatMicros,
  formatTokens,
} from "@/lib/format";
import type { Step, StepKind } from "@/lib/api/client";

/**
 * What each step says about itself.
 *
 * The trail is read by an operator working out what to do next and by an
 * auditor working out what happened, so every line states a fact with its
 * number rather than a category: "R$ 0,003 consumidos · R$ 0,047 devolvidos",
 * never "orçamento atualizado".
 */

const TITLES: Record<StepKind, string> = {
  run_started: "runs.storyStarted",
  planned: "runs.storyProposed",
  gate_decided: "runs.storyGateDecided",
  budget_reserved: "runs.storyBudgetReserved",
  tool_called: "runs.storyToolCalled",
  tool_returned: "runs.storyToolReturned",
  budget_reconciled: "runs.storyBudgetReconciled",
  approval_requested: "runs.storyAwaitingHuman",
  approval_decided: "runs.storyHumanDecided",
  resumed: "runs.storyResumed",
  abandoned: "runs.storyAbandoned",
  compensated: "runs.nodeCompensated",
  failed: "runs.storyFailed",
  parked: "runs.storyParked",
  run_finished: "runs.storyFinished",
};

const VERDICT_CHIP: Record<string, { text: string; className: string }> = {
  allow: { text: "verdict.allow", className: "bg-success-surface text-success" },
  constrain: {
    text: "verdict.constrain",
    className: "bg-warning-surface text-warning",
  },
  require_approval: {
    text: "verdict.require_approval",
    className: "bg-warning-surface text-warning",
  },
  block: { text: "verdict.block", className: "bg-danger-surface text-danger" },
};

const PARKED: Record<string, string> = {
  budget_exhausted: "runs.ceilingHit",
  no_progress: "runs.storyInsisted",
};

export interface Chip {
  text: string;
  pill?: boolean;
  className?: string;
}

export function titleOf(step: Step): string {
  return TITLES[step.kind];
}

export function chipsOf(step: Step): Chip[] {
  const payload = (step.payload ?? {}) as Record<string, unknown>;
  const chips: Chip[] = [];

  if (typeof payload.tool === "string") chips.push({ text: payload.tool });
  const effect = effectOf(step);
  if (effect && effect !== "read" && step.kind === "tool_called") {
    chips.push({
      text: effect,
      pill: true,
      className: "bg-warning-surface text-warning",
    });
  }
  if (typeof payload.model === "string") chips.push({ text: payload.model });

  const verdict = verdictOf(step);
  if (verdict && VERDICT_CHIP[verdict]) {
    chips.push({ ...VERDICT_CHIP[verdict], pill: true });
  }
  for (const label of step.labels ?? []) {
    chips.push({
      text: label,
      pill: true,
      className: "bg-warning-surface text-warning",
    });
  }
  return chips;
}

/**
 * A line to render: which sentence, and what to put in it.
 *
 * Not a formatted string, because this module has no React context and cannot
 * translate. Returning the words would mean returning them in one language —
 * which is what it did, and why an English installation read "gatilho cron"
 * under an English title.
 */
export type Line = { key: string; values?: Record<string, unknown> };

/** Nothing to say. */
const NOTHING: Line = { key: "" };

export function detailOf(step: Step): Line {
  const payload = (step.payload ?? {}) as Record<string, unknown>;

  switch (step.kind) {
    case "run_started":
      return typeof payload.trigger === "string"
        ? { key: "runs.storyTrigger", values: { trigger: payload.trigger } }
        : NOTHING;

    case "gate_decided": {
      // Never "runs.storyRefused": the rule is named and explained, so the
      // reader knows what to change. An allowed call has no rule to explain —
      // it says which effect was inside which pack, which is the fact an
      // auditor is checking.
      const rule = typeof payload.rule === "string" ? payload.rule : "";
      const budget = budgetLine(payload);
      if (rule === "budget" && budget) return budget;
      const explained = explainRule(rule);
      if (explained) return { key: explained };
      const effect = effectOf(step);
      return effect
        ? { key: "runs.storyEffectInPack", values: { effect } }
        : NOTHING;
    }

    case "budget_reserved":
      return typeof payload.micros === "number"
        ? { key: "runs.storyHeld", values: { amount: formatMicros(payload.micros) } }
        : NOTHING;

    case "budget_reconciled": {
      const released =
        typeof payload.released_micros === "number"
          ? payload.released_micros
          : 0;
      return {
        key: "runs.storySpentReleased",
        values: { spent: formatCost(step.cost), released: formatMicros(released) },
      };
    }

    case "tool_returned":
      if (payload.failed) {
        return { key: "runs.storyToolFailed", values: { code: payload.error_code ?? "" } };
      }
      if (payload.cached) {
        return {
          key: "runs.storyToolCached",
          values: {
            run: payload.cached_from_run ?? "",
            seq: payload.cached_from_seq ?? "",
          },
        };
      }
      return NOTHING;

    case "approval_decided":
      return {
        key: payload.approved ? "runs.storyApprovedBy" : "runs.storyRefusedBy",
        values: { who: payload.by ?? "—" },
      };

    case "parked": {
      const reason = typeof payload.reason === "string" ? payload.reason : "";
      // A reason this console does not know is shown as it came. The trail
      // outlives the console, and hiding a word it has not learned yet would
      // be the console editing history.
      return { key: PARKED[reason] ?? reason };
    }

    case "failed":
      return {
        key: typeof payload.message === "string"
          ? payload.message
          : String(payload.code ?? ""),
      };

    case "run_finished": {
      // The step's own exception, when the agent said that is why it stopped.
      // Quoted rather than paraphrased: they are the author's words, and the
      // trail says the agent asserted them rather than that anything checked.
      const stopped =
        typeof payload.stopped_by === "string" ? payload.stopped_by : "";
      // Two eras. A run recorded before the answer moved carries it inline; one
      // recorded since carries a reference, because the answer restates what
      // the agent read and had to live where an erasure can reach it. Rendering
      // the second as an empty line would report that the agent finished
      // silently.
      const inline =
        typeof payload.outcome === "string" ? payload.outcome : "";
      const held = typeof payload.outcome_ref === "string" && payload.outcome_ref;
      const reason = typeof payload.reason === "string" ? payload.reason : "";
      if (stopped) {
        const outcome = inline;
        return { key: "runs.stoppedByException", values: { what: stopped, outcome } };
      }
      if (reason === "no_tool_call") {
        return inline
          ? { key: "runs.finishedByNoToolCallWithOutcome", values: { outcome: inline } }
          : held
            ? { key: "runs.finishedByNoToolCallStored" }
            : { key: "runs.finishedByNoToolCall" };
      }
      return inline
        ? { key: "runs.finishedWithOutcome", values: { outcome: inline } }
        : held
          ? { key: "runs.outcomeStored" }
          : NOTHING;
    }

    default:
      return NOTHING;
  }
}

function budgetLine(payload: Record<string, unknown>): Line | undefined {
  const breached = typeof payload.breached === "string" ? payload.breached : "";
  const budget = record(payload.budget);
  const committed = record(payload.committed);
  const estimate = record(payload.estimate);
  const projected = record(payload.projected);
  if (!breached || !budget || !projected) return undefined;

  const dim = budgetDimension(breached);
  if (!dim) return undefined;

  const ceiling = dim.read(budget);
  const used = dim.read(projected);
  if (ceiling <= 0 || used <= 0) return undefined;

  return {
    key: dim.key,
    values: {
      used: dim.format(used),
      ceiling: dim.format(ceiling),
      already: dim.format(dim.read(committed)),
      requested: dim.format(dim.read(estimate)),
    },
  };
}

type RawRecord = Record<string, unknown>;

function record(value: unknown): RawRecord {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as RawRecord)
    : {};
}

function numberField(value: RawRecord, key: string): number {
  const found = value[key];
  return typeof found === "number" && Number.isFinite(found) ? found : 0;
}

function budgetDimension(name: string):
  | {
      key: string;
      read: (value: RawRecord) => number;
      format: (value: number) => string;
    }
  | undefined {
  switch (name) {
    case "cost":
      return {
        key: "runs.storyBudgetExceededCost",
        read: (value) => numberField(value, "micros"),
        format: formatMicros,
      };
    case "tokens":
      return {
        key: "runs.storyBudgetExceededTokens",
        read: (value) => numberField(value, "tokens"),
        format: formatTokens,
      };
    case "tool calls":
      return {
        key: "runs.storyBudgetExceededToolCalls",
        read: (value) => numberField(value, "tool_calls"),
        format: formatTokens,
      };
    case "steps":
      return {
        key: "runs.storyBudgetExceededSteps",
        read: (value) => numberField(value, "steps"),
        format: formatTokens,
      };
    case "wall clock":
      return {
        key: "runs.storyBudgetExceededWallClock",
        read: (value) => numberField(value, "wall_clock_ms"),
        format: formatDurationMs,
      };
    default:
      return undefined;
  }
}

/** The one-line summary a folded step gets in the opened list. */
export function summaryOf(step: Step): Line {
  const payload = (step.payload ?? {}) as Record<string, unknown>;
  const tool = typeof payload.tool === "string" ? payload.tool : "";
  const verdict = verdictOf(step);

  switch (step.kind) {
    case "planned":
      return { key: "runs.summaryProposed", values: { tool } };
    case "gate_decided":
      return {
        key: verdict === "allow" ? "runs.summaryAllowed" : "runs.summaryDecided",
        values: { tool },
      };
    case "tool_called":
      return { key: "runs.summaryToolCalled", values: { tool } };
    case "tool_returned":
      return payload.cached
        ? { key: "runs.summaryToolCached", values: { tool } }
        : { key: "runs.summaryToolReturned", values: { tool } };
    case "budget_reserved":
      return typeof payload.tokens === "number"
        ? {
            key: "runs.summaryReservedTokens",
            values: { tokens: formatTokens(payload.tokens) },
          }
        : { key: "runs.storyBudgetReserved" };
    case "budget_reconciled":
      return {
        key: "runs.storyReconciledWith",
        values: { spent: formatCost(step.cost) },
      };
    default:
      return { key: TITLES[step.kind] };
  }
}

/**
 * Renders a line. Empty key renders nothing, which is how a step with nothing
 * to add says so.
 *
 * Here rather than at each call site because there are three of them and the
 * shape is the module's, not the screen's.
 */
export function line(
  { key, values }: Line,
  t: (key: string, values?: Record<string, unknown>) => string,
): string {
  return key ? t(key, values) : "";
}
