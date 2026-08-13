import { effectOf, verdictOf } from "@/features/runs/step-verb";
import { explainRule } from "@/lib/gate-rules";
import { formatCost, formatMicros, formatTokens } from "@/lib/format";
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
  budget_reserved: "Orçamento reservado",
  tool_called: "runs.storyToolCalled",
  tool_returned: "runs.storyToolReturned",
  budget_reconciled: "Orçamento reconciliado",
  approval_requested: "runs.storyAwaitingHuman",
  approval_decided: "runs.storyHumanDecided",
  abandoned: "runs.storyAbandoned",
  compensated: "runs.nodeCompensated",
  failed: "runs.storyFailed",
  parked: "runs.storyParked",
  run_finished: "runs.storyFinished",
};

const VERDICT_CHIP: Record<string, { text: string; className: string }> = {
  allow: { text: "permitir", className: "bg-success-surface text-success" },
  constrain: {
    text: "restringir",
    className: "bg-warning-surface text-warning",
  },
  require_approval: {
    text: "escalar",
    className: "bg-warning-surface text-warning",
  },
  block: { text: "bloquear", className: "bg-danger-surface text-danger" },
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

export function detailOf(step: Step): string {
  const payload = (step.payload ?? {}) as Record<string, unknown>;

  switch (step.kind) {
    case "run_started":
      return typeof payload.trigger === "string"
        ? `gatilho ${payload.trigger}`
        : "";

    case "gate_decided": {
      // Never "runs.storyRefused": the rule is named and explained, so the
      // reader knows what to change. An allowed call has no rule to explain —
      // it says which effect was inside which pack, which is the fact an
      // auditor is checking.
      const rule = typeof payload.rule === "string" ? payload.rule : "";
      const explained = explainRule(rule);
      if (explained) return explained;
      const effect = effectOf(step);
      return effect ? `efeito ${effect} dentro do pacote da execução` : "";
    }

    case "budget_reserved":
      return typeof payload.micros === "number"
        ? `${formatMicros(payload.micros)} retidos do teto da execução`
        : "";

    case "budget_reconciled": {
      const released =
        typeof payload.released_micros === "number"
          ? payload.released_micros
          : 0;
      return `${formatCost(step.cost)} consumidos · ${formatMicros(released)} devolvidos`;
    }

    case "tool_returned":
      return payload.failed ? `falhou: ${payload.error_code ?? "erro"}` : "";

    case "approval_decided":
      return payload.approved
        ? `aprovada por ${payload.by ?? "—"}`
        : `recusada por ${payload.by ?? "—"}`;

    case "parked": {
      const reason = typeof payload.reason === "string" ? payload.reason : "";
      return PARKED[reason] ?? reason;
    }

    case "failed":
      return typeof payload.message === "string"
        ? payload.message
        : String(payload.code ?? "");

    case "run_finished":
      return typeof payload.outcome === "string" ? payload.outcome : "";

    default:
      return "";
  }
}

/** The one-line summary a folded step gets in the opened list. */
export function summaryOf(step: Step): string {
  const payload = (step.payload ?? {}) as Record<string, unknown>;
  const tool = typeof payload.tool === "string" ? ` ${payload.tool}` : "";
  const verdict = verdictOf(step);

  switch (step.kind) {
    case "planned":
      return `Modelo propôs${tool}`;
    case "gate_decided":
      return `Portão ${verdict === "allow" ? "permitiu" : "decidiu"}${tool}`;
    case "tool_called":
      return `Ferramenta chamada${tool}`;
    case "tool_returned":
      return `Ferramenta respondeu${tool}`;
    case "budget_reserved":
      return `Orçamento reservado${
        typeof payload.tokens === "number"
          ? ` · ${formatTokens(payload.tokens)} tokens`
          : ""
      }`;
    case "budget_reconciled":
      return `Orçamento reconciliado · ${formatCost(step.cost)}`;
    default:
      return TITLES[step.kind];
  }
}
