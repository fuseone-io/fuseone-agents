import { Badge } from "@/components/ui/badge";
import { Mono } from "@/components/shared/mono";
import { cn } from "@/lib/utils";
import { formatCost, formatInstant, shortHash } from "@/lib/format";
import { explainRule } from "@/lib/gate-rules";
import { TONE_DOT, TONE_TEXT, verbOf } from "@/features/runs/step-verb";
import type { Step } from "@/lib/api/client";

/**
 * One act in the run's trail.
 *
 * The spine is the point: a run is a sequence, and a table of rows reads as a
 * set. The dot and the line are what make the order legible at a glance.
 */
export function StepRow({ step, last }: { step: Step; last: boolean }) {
  const { verb, tone } = verbOf(step);
  const payload = (step.payload ?? {}) as Record<string, unknown>;
  const explanation =
    typeof payload.rule === "string" ? explainRule(payload.rule) : "";

  return (
    <li className="grid grid-cols-[16px_1fr] gap-3">
      <div className="flex flex-col items-center">
        <span
          aria-hidden
          className={cn(
            "mt-1.5 size-[7px] shrink-0 rounded-pill",
            TONE_DOT[tone],
          )}
        />
        {!last && <span aria-hidden className="w-px flex-1 bg-border" />}
      </div>

      <div className="min-w-0 pb-4">
        <div className="flex flex-wrap items-baseline gap-2">
          <Mono dim>{formatInstant(step.at)}</Mono>
          <Mono className={TONE_TEXT[tone]}>{verb}</Mono>
          {typeof payload.tool === "string" && <Mono>{payload.tool}</Mono>}
          {step.cost && step.cost.micros > 0 && (
            <Mono dim>{formatCost(step.cost)}</Mono>
          )}
        </div>

        {/* The trail never reads "denied by policy": it names the rule that
            fired and explains it, so the reader knows what to change. */}
        {explanation && (
          <p className="mt-1 text-sm text-text-secondary">{explanation}</p>
        )}

        {step.labels && step.labels.length > 0 && (
          <div className="mt-1.5 flex flex-wrap gap-1">
            {step.labels.map((label) => (
              <Badge key={label} variant="secondary" className="font-normal">
                {label}
              </Badge>
            ))}
          </div>
        )}

        <div className="mt-1 flex flex-wrap items-baseline gap-2 text-2xs text-muted-foreground">
          <Mono dim>#{step.seq}</Mono>
          {/* The hash is what makes the step checkable, so it is on the step
              rather than only in the verification result. */}
          <Mono dim>
            <span title={step.hash}>{shortHash(step.hash)}</span>
          </Mono>
        </div>
      </div>
    </li>
  );
}
