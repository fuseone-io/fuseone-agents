import { useTranslation } from "react-i18next";
import { ChevronDown } from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Mono } from "@/components/shared/mono";
import { ActRow } from "@/features/agents/simulation-act";
import { SettledBadge } from "@/features/agents/simulation-settled";
import { reasonKey } from "@/features/agents/simulation-reason";
import { formatMicros } from "@/lib/format";
import type { SimulationCase } from "@/features/agents/simulation-api";

/**
 * One occurrence, and what the agent would have done with it.
 *
 * Collapsed by default because the answer an author wants first is how many
 * cases ended badly, not what each one proposed. The proposals are one press
 * away, and that press is what a correction is written from (FU-12).
 */
export function CaseRow({
  index,
  entry,
  onCorrect,
}: {
  index: number;
  entry: SimulationCase;
  /** Absent when the case has no run to correct from. */
  onCorrect?: () => void;
}) {
  const { t } = useTranslation();
  const acts = entry.acted ?? [];
  const unmet = entry.unmet ?? [];

  return (
    <Collapsible className="overflow-hidden rounded-xl border bg-card shadow-sm">
      {/* The correction sits beside the trigger, not inside it. A control
          nested in a control is invalid markup, and the hand-rolled span it
          would take instead loses the focus ring and the keyboard behaviour
          that make it usable by anybody not holding a mouse. */}
      <div className="flex items-center">
        <CollapsibleTrigger asChild>
          <Button
            variant="ghost"
            className="flex h-auto flex-1 items-center justify-start gap-3 rounded-none px-4 py-3 text-left"
          >
            <span className="text-sm text-muted-foreground tabular-nums">
              {t("simulation.caseNumber", { n: index })}
            </span>
            <SettledBadge settled={entry.settled} />

            <span className="text-xs text-muted-foreground">
              {t("simulation.stepCount", { count: entry.steps })}
            </span>
            <Mono className="text-xs text-muted-foreground">
              {formatMicros(entry.cost.micros)}
            </Mono>

            {entry.reason && <Reason reason={entry.reason} />}

            {entry.expected && entry.expected.length > 0 && (
              <Badge variant={unmet.length > 0 ? "destructive" : "secondary"}>
                {unmet.length > 0
                  ? t("correction.broke", { count: unmet.length })
                  : t("correction.held")}
              </Badge>
            )}

            <ChevronDown
              aria-hidden
              className="ml-auto size-4 shrink-0 text-muted-foreground transition-transform [[data-state=open]_&]:rotate-180"
            />
          </Button>
        </CollapsibleTrigger>

        {onCorrect && (
          <Button
            variant="ghost"
            size="sm"
            className="mr-2 h-7 shrink-0 text-muted-foreground"
            onClick={onCorrect}
          >
            {t("correction.correct")}
          </Button>
        )}
      </div>

      <CollapsibleContent>
        {acts.length === 0 ? (
          <p className="border-t px-4 py-3 text-xs text-muted-foreground">
            {t("simulation.noActs")}
          </p>
        ) : (
          <ul>
            {acts.map((act, i) => (
              <ActRow key={`${act.tool}-${i}`} act={act} />
            ))}
          </ul>
        )}
      </CollapsibleContent>
    </Collapsible>
  );
}

/** The stable code as a sentence, or as it came when nobody translated it. */
function Reason({ reason }: { reason: string }) {
  const { t } = useTranslation();
  const key = reasonKey(reason);
  if (!key) {
    return <Mono className="text-2xs text-muted-foreground">{reason}</Mono>;
  }
  return <span className="text-xs text-muted-foreground">{t(key)}</span>;
}
