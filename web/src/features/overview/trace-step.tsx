import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { TONE_DOT, TONE_TEXT, verbOf } from "@/features/runs/step-verb";
import { titleOf, chipsOf } from "@/features/runs/step-story";
import { formatTime } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Step } from "@/lib/api/client";

/**
 * One step, at the width of a rail.
 *
 * The full trail's row carries chips, latency, a hash and an expandable
 * payload. None of that fits in 340px, and cramming it would produce a
 * worse version of a screen that already exists — so this says what happened
 * and when, and the run's own page is one click away for the rest.
 */
export function TraceStep({ step, last }: { step: Step; last: boolean }) {
  const { t } = useTranslation();
  const { tone } = verbOf(step);
  const tool = chipsOf(step).find((chip) => !chip.pill);

  return (
    <li className="grid grid-cols-[10px_1fr] gap-2">
      <div className="flex flex-col items-center">
        <span
          aria-hidden
          className={cn(
            "mt-1.5 size-[6px] shrink-0 rounded-pill",
            TONE_DOT[tone],
          )}
        />
        {!last && <span aria-hidden className="w-px flex-1 bg-border" />}
      </div>

      <div className="min-w-0 pb-2">
        <div className="flex items-baseline gap-1.5">
          <Mono dim className="text-2xs">
            {formatTime(step.at)}
          </Mono>
          <span className={cn("truncate text-xs", TONE_TEXT[tone])}>
            {t(titleOf(step))}
          </span>
        </div>
        {tool && (
          <Mono dim className="block truncate text-2xs">
            {tool.text}
          </Mono>
        )}
      </div>
    </li>
  );
}
