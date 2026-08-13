import { useTranslation } from "react-i18next";
import { useState } from "react";
import { ChevronDown } from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Mono } from "@/components/shared/mono";
import { StepContent } from "@/features/runs/step-content";
import { TILE, tileOf } from "@/features/runs/trail-icon";
import { titleOf, detailOf, chipsOf, line } from "@/features/runs/step-story";
import { cn } from "@/lib/utils";
import { formatTime, shortHash } from "@/lib/format";
import type { Step } from "@/lib/api/client";

/**
 * One event in the trail.
 *
 * The tile and its connector are what make a run read as a sequence rather
 * than a table of records — the eye follows the line and stops where the
 * colour changes. Events that reference content open; the rest do not pretend
 * to.
 */
export function TrailEvent({
  runId,
  step,
  live,
  last,
  showHashes,
}: {
  runId: string;
  step: Step;
  live: boolean;
  last: boolean;
  showHashes: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const { icon: Icon, tone } = tileOf(step, live);
  const expandable = hasContent(step);

  const body = (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium">{t(titleOf(step))}</span>
        {chipsOf(step).map((chip) => (
          <span
            key={chip.text}
            className={cn(
              "rounded-md px-1.5 font-mono text-2xs tabular-nums",
              chip.pill ? "rounded-pill px-2 font-medium" : "",
              chip.className ?? "bg-muted text-foreground",
            )}
          >
            {t(chip.text)}
          </span>
        ))}
        {expandable && (
          <ChevronDown
            aria-hidden
            className={cn(
              "size-3.5 text-muted-foreground transition-transform",
              open && "rotate-180",
            )}
          />
        )}
      </div>
      {line(detailOf(step), t) && (
        <p className="mt-0.5 text-xs text-muted-foreground">
          {line(detailOf(step), t)}
        </p>
      )}
    </>
  );

  return (
    <li
      id={`step-${step.seq}`}
      className="grid grid-cols-[26px_1fr_auto] gap-x-3"
    >
      <div className="flex flex-col items-center">
        <span
          className={cn(
            "flex size-6 items-center justify-center rounded-lg border",
            TILE[tone],
          )}
        >
          <Icon className="size-3.5" aria-hidden />
        </span>
        {!last && <span aria-hidden className="w-px flex-1 bg-border" />}
      </div>

      <div className="min-w-0 pb-4 pt-0.5">
        {expandable ? (
          <Collapsible open={open} onOpenChange={setOpen}>
            <CollapsibleTrigger className="w-full rounded-lg text-left focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/26">
              {body}
            </CollapsibleTrigger>
            <CollapsibleContent>
              <StepContent runId={runId} seq={step.seq} open={open} />
            </CollapsibleContent>
          </Collapsible>
        ) : (
          body
        )}
      </div>

      <div className="pt-1 text-right">
        <Mono dim className="text-2xs">
          #{step.seq} · {formatTime(step.at)}
        </Mono>
        {showHashes && (
          <div>
            <Mono className="text-2xs text-text-disabled">
              <span title={step.hash}>{shortHash(step.hash)}</span>
            </Mono>
          </div>
        )}
      </div>
    </li>
  );
}

/** Only steps that reference stored content have anything to open. */
function hasContent(step: Step): boolean {
  const payload = (step.payload ?? {}) as Record<string, unknown>;
  return Boolean(payload.args_ref || payload.result_ref || payload.input_ref);
}
