import { useTranslation } from "react-i18next";
import { ChevronDown } from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Button } from "@/components/ui/button";
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
}: {
  index: number;
  entry: SimulationCase;
}) {
  const { t } = useTranslation();
  const acts = entry.acted ?? [];

  return (
    <Collapsible className="overflow-hidden rounded-xl border bg-card shadow-sm">
      <CollapsibleTrigger asChild>
        <Button
          variant="ghost"
          className="flex h-auto w-full items-center justify-start gap-3 rounded-none px-4 py-3 text-left"
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

          <ChevronDown
            aria-hidden
            className="ml-auto size-4 shrink-0 text-muted-foreground transition-transform [[data-state=open]_&]:rotate-180"
          />
        </Button>
      </CollapsibleTrigger>

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
