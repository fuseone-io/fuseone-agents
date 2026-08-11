import { useState } from "react";
import { Ellipsis } from "lucide-react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Mono } from "@/components/shared/mono";
import { summaryOf } from "@/features/runs/step-story";
import { formatDurationMs } from "@/lib/format";
import type { Step } from "@/lib/api/client";

/**
 * A stretch of routine steps, folded into one row.
 *
 * The fold states how many steps it holds and what they were, and opens to a
 * list of all of them: nothing is hidden, only ranked. An eighteen-event run
 * that reads as eighteen identical blocks buries the two decisions that matter
 * among the bookkeeping that does not.
 */
export function TrailFold({ steps, last }: { steps: Step[]; last: boolean }) {
  const [open, setOpen] = useState(false);
  const first = steps[0];
  const final = steps[steps.length - 1];
  const elapsed = Date.parse(final.at) - Date.parse(first.at);

  return (
    <li className="grid grid-cols-[26px_1fr_auto] gap-x-3">
      <div className="flex flex-col items-center">
        <span className="flex size-6 items-center justify-center rounded-lg border border-dashed border-border-strong bg-muted text-muted-foreground">
          <Ellipsis className="size-3.5" aria-hidden />
        </span>
        {!last && <span aria-hidden className="w-px flex-1 bg-border" />}
      </div>

      <div className="min-w-0 pb-4 pt-1">
        <Collapsible open={open} onOpenChange={setOpen}>
          <CollapsibleTrigger className="rounded-lg text-left text-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/26">
            {steps.length} passos de rotina · leituras permitidas, sem alteração de estado
          </CollapsibleTrigger>
          <CollapsibleContent>
            <ul className="mt-2 flex flex-col gap-1.5">
              {steps.map((step) => (
                <li key={step.seq} className="flex gap-2.5 text-xs text-muted-foreground">
                  <Mono dim className="text-2xs">
                    #{step.seq}
                  </Mono>
                  {summaryOf(step)}
                </li>
              ))}
            </ul>
          </CollapsibleContent>
        </Collapsible>
      </div>

      <div className="pt-1.5 text-right">
        <Mono dim className="text-2xs">
          #{first.seq} – #{final.seq} · {formatDurationMs(elapsed)}
        </Mono>
      </div>
    </li>
  );
}
