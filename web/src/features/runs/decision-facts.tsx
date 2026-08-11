import type { ReactNode } from "react";
import { Mono } from "@/components/shared/mono";
import { effectOf } from "@/features/runs/step-verb";
import { formatMicros } from "@/lib/format";
import type { PendingApproval, Step } from "@/lib/api/client";

const EFFECT_RISK: Record<string, { label: string; className: string }> = {
  read: { label: "baixo", className: "text-muted-foreground" },
  write: { label: "médio", className: "text-warning" },
  financial: { label: "alto", className: "text-danger" },
  irreversible: { label: "alto", className: "text-danger" },
};

/**
 * The three things that decide the answer: which rule stopped the call, how
 * much damage it can do, and what it will cost.
 *
 * Risk is read off the effect rather than stored: an effect is what the tool
 * can do to the world, which is exactly what an approver is weighing.
 */
export function DecisionFacts({
  approval,
  step,
}: {
  approval: PendingApproval;
  step?: Step;
}) {
  const payload = (step?.payload ?? {}) as { estimate?: { micros?: number } };
  // The run summary carries the effect by name; the step's payload carries the
  // domain's integer. Reading the raw payload put "2" on screen where the
  // approver needed the word.
  const effect = approval.effect ?? (step && effectOf(step)) ?? "read";
  const risk = EFFECT_RISK[effect] ?? EFFECT_RISK.read;
  const micros = payload.estimate?.micros;

  return (
    <dl className="flex flex-col gap-2.5">
      <Fact label="Regra">
        <span className="text-sm">{approval.rule ?? "—"}</span>
      </Fact>
      <Fact label="Efeito">
        <Mono>{effect}</Mono>
      </Fact>
      <Fact label="Risco">
        <span className={`text-sm ${risk.className}`}>{risk.label}</span>
      </Fact>
      <Fact label="Custo estimado">
        {/* No estimate is not zero cost. Printing R$ 0,00 would be a claim the
            platform cannot make about a call it has not run. */}
        <Mono>{micros === undefined ? "—" : formatMicros(micros)}</Mono>
      </Fact>
    </dl>
  );
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}
