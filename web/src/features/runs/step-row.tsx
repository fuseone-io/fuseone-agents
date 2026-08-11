import {
  BadgeCheck,
  CircleDollarSign,
  Flag,
  Hand,
  PauseCircle,
  Play,
  Sparkles,
  Undo2,
  Wrench,
  XCircle,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import type { Step, StepKind } from "@/lib/api/client";
import { formatCost, formatInstant, shortHash } from "@/lib/format";
import { explainRule } from "@/lib/gate-rules";

// Kind is a closed set in the contract, so this map is exhaustive by
// construction: adding a kind to the API makes this fail to type-check.
const KINDS: Record<StepKind, { label: string; icon: LucideIcon }> = {
  run_started: { label: "Execução iniciada", icon: Play },
  planned: { label: "Modelo propôs", icon: Sparkles },
  gate_decided: { label: "Portão decidiu", icon: BadgeCheck },
  budget_reserved: { label: "Orçamento reservado", icon: CircleDollarSign },
  tool_called: { label: "Ferramenta chamada", icon: Wrench },
  tool_returned: { label: "Ferramenta respondeu", icon: Wrench },
  budget_reconciled: { label: "Orçamento conciliado", icon: CircleDollarSign },
  approval_requested: { label: "Aprovação solicitada", icon: Hand },
  approval_decided: { label: "Aprovação decidida", icon: BadgeCheck },
  compensated: { label: "Ação revertida", icon: Undo2 },
  failed: { label: "Falhou", icon: XCircle },
  parked: { label: "Estacionada", icon: PauseCircle },
  run_finished: { label: "Execução concluída", icon: Flag },
};

// A verdict is a decision, so it renders as a pill in the status colour on its
// matching surface — the shape the design system reserves for an outcome — and
// always with the word, never the colour alone.
const VERDICTS: Record<string, { label: string; className: string }> = {
  allow: { label: "permitir", className: "bg-success-surface text-success" },
  constrain: { label: "restringir", className: "bg-info-surface text-info" },
  require_approval: { label: "aprovar", className: "bg-warning-surface text-warning" },
  block: { label: "bloquear", className: "bg-danger-surface text-danger" },
};

// The wire encodes verdict as the domain's integer; map it back for display.
const VERDICT_BY_CODE = ["unknown", "allow", "constrain", "require_approval", "block"];

export function StepRow({ step }: { step: Step }) {
  const kind = KINDS[step.kind];
  const Icon = kind.icon;
  const payload = (step.payload ?? {}) as Record<string, unknown>;
  const verdict = verdictOf(payload);

  return (
    <li className="flex gap-3 border-b px-4 py-3 last:border-b-0">
      <span className="mt-0.5 text-muted-foreground" aria-hidden>
        <Icon className="size-4" />
      </span>

      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm font-medium">{kind.label}</span>
          {typeof payload.tool === "string" && (
            <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">{payload.tool}</code>
          )}
          {verdict && (
            <Badge
              variant="outline"
              className={`rounded-pill border-transparent font-mono text-2xs font-normal ${verdict.className}`}
            >
              {verdict.label}
            </Badge>
          )}
          {typeof payload.rule === "string" && (
            <span className="text-xs text-muted-foreground">regra: {payload.rule}</span>
          )}
        </div>

        {/* The trail never reads "denied by policy": it names the rule that
            fired and explains it, so the reader knows what to change. */}
        {typeof payload.rule === "string" && explainRule(payload.rule) && (
          <p
            className="mt-1 text-sm text-muted-foreground"
            title={typeof payload.reason === "string" ? payload.reason : undefined}
          >
            {explainRule(payload.rule)}
          </p>
        )}

        {step.labels && step.labels.length > 0 && (
          <div className="mt-1 flex gap-1">
            {step.labels.map((label) => (
              <Badge key={label} variant="secondary" className="font-normal">
                {label}
              </Badge>
            ))}
          </div>
        )}
      </div>

      <div className="shrink-0 text-right text-xs text-muted-foreground">
        <div className="tabular-nums">#{step.seq}</div>
        <div className="tabular-nums">{formatInstant(step.at)}</div>
        {step.cost && step.cost.micros > 0 && (
          <div className="tabular-nums">{formatCost(step.cost)}</div>
        )}
        <div className="font-mono" title={step.hash}>
          {shortHash(step.hash)}
        </div>
      </div>
    </li>
  );
}

function verdictOf(payload: Record<string, unknown>) {
  const raw = payload.verdict;
  const name = typeof raw === "number" ? VERDICT_BY_CODE[raw] : raw;
  return typeof name === "string" ? VERDICTS[name] : undefined;
}
