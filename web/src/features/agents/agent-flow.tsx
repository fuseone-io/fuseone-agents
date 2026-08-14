import { useTranslation } from "react-i18next";
import { CircleAlert, CircleDot, Wrench } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { StepStrip } from "@/features/agents/step-strip";
import { Mono } from "@/components/shared/mono";
import type { components } from "@/lib/api/schema.gen";

type AgentStep = components["schemas"]["AgentStep"];

/**
 * The process the definition declares, as a sequence.
 *
 * A projection of the specification and never a second copy of it: the
 * ordering is the order the author wrote, so the same version draws the same
 * thing every time it is read (FU-17, FU-18).
 *
 * A step is not a tool call. One that reaches nothing is the agent thinking,
 * and drawing that as an empty box is the point rather than a gap — the
 * capability pack is the ceiling and the step is the narrower permission.
 */
export function AgentFlow({ steps }: { steps: AgentStep[] }) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-3">
      {/* The same strip the author arranged, drawn the same way. Nothing
          about it was stored: a sequence and a rule between its cards, so an
          approver and an auditor two years apart see one picture (FU-17,
          FU-18). */}
      <div className="overflow-hidden rounded-lg border border-border">
        <StepStrip
          steps={steps}
          stops={() => false}
          onSelect={() => undefined}
          onAdd={() => undefined}
        />
      </div>

      <ol className="flex flex-col">
      {steps.map((step, at) => (
        <li key={`${step.name}-${at}`} className="flex gap-3">
          <Rail last={at === steps.length - 1} index={at + 1} />
          <div className="min-w-0 flex-1 pb-5">
            <p className="text-sm font-medium">{step.name}</p>

            <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
              {(step.reaches ?? []).length === 0 ? (
                <span className="text-2xs text-muted-foreground">
                  {t("agents.reachesNothing")}
                </span>
              ) : (
                (step.reaches ?? []).map((tool) => (
                  <Badge key={tool} variant="outline" className="gap-1">
                    <Wrench className="size-3" aria-hidden />
                    <Mono className="text-2xs">{tool}</Mono>
                  </Badge>
                ))
              )}
              {step.model && (
                <Badge variant="secondary" className="text-2xs">
                  {t("agents.stepModel", { model: step.model })}
                </Badge>
              )}
            </div>

            {step.stopsWhen && (
              <p className="mt-1.5 flex items-start gap-1.5 text-2xs text-muted-foreground">
                <CircleAlert className="mt-px size-3.5 shrink-0" aria-hidden />
                {t("agents.stopsWhen", { what: step.stopsWhen })}
              </p>
            )}
          </div>
        </li>
      ))}
      </ol>
    </div>
  );
}

/** The line that makes a list read as a sequence. */
function Rail({ index, last }: { index: number; last: boolean }) {
  return (
    <div className="flex flex-col items-center">
      <span className="flex size-6 shrink-0 items-center justify-center rounded-md border border-border bg-muted text-2xs tabular-nums text-muted-foreground">
        {index}
      </span>
      {!last && <span className="w-px flex-1 bg-border" />}
      {last && <CircleDot className="mt-1 size-3 text-muted-foreground" aria-hidden />}
    </div>
  );
}
