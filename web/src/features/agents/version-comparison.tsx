import { useTranslation } from "react-i18next";
import { ArrowRight } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Mono } from "@/components/shared/mono";
import { Panel } from "@/components/shared/panel";
import { useComparison } from "@/features/agents/simulation-api";
import { formatMicros } from "@/lib/format";
import type { CaseChange } from "@/features/agents/simulation-api";

/**
 * Is this version better than the one before it?
 *
 * The question somebody is actually asking when they publish, and the one two
 * reports side by side do not answer. Absent entirely when one of the two
 * versions was never run against the corrections: there is nothing to say,
 * and an empty table would say "nothing changed".
 */
export function VersionComparison({ agentId }: { agentId: string }) {
  const { t } = useTranslation();
  const { data, error } = useComparison(agentId);

  if (error || !data || data.cases.length === 0) return null;

  return (
    <Panel
      title={t("comparison.title")}
      action={
        <span className="flex items-center gap-1.5 text-2xs text-muted-foreground">
          <Mono dim>{data.from.slice(0, 9)}</Mono>
          <ArrowRight className="size-3" aria-hidden />
          <Mono dim>{data.to.slice(0, 9)}</Mono>
        </span>
      }
      flush
    >
      <p className="flex flex-wrap items-center gap-x-4 gap-y-1 px-4 pb-3 text-sm">
        <span>{t("comparison.regressed", { count: data.regressed })}</span>
        <span>{t("comparison.fixed", { count: data.fixed })}</span>
        {/* Money moves without a single correction breaking, and that is the
            regression a held-and-broken count reports as no change. */}
        <span className="text-muted-foreground">
          {t(
            data.costMicros < 0
              ? "comparison.costMovedDown"
              : "comparison.costMovedUp",
            { amount: formatMicros(Math.abs(data.costMicros)) },
          )}
        </span>
      </p>

      <ul className="flex flex-col">
        {data.cases.map((change) => (
          <ChangeRow key={change.id} change={change} />
        ))}
      </ul>
    </Panel>
  );
}

/**
 * Written out rather than built from the value, so the guard that checks every
 * key exists can see them: a template literal is invisible to it, and a key
 * nobody can find is a key nobody translates.
 */
const STANDING: Record<CaseChange["was"], string> = {
  held: "comparison.standing.held",
  broke: "comparison.standing.broke",
  absent: "comparison.standing.absent",
};

function ChangeRow({ change }: { change: CaseChange }) {
  const { t } = useTranslation();
  const regressed = change.was === "held" && change.now === "broke";

  return (
    <li className="flex items-center gap-3 border-b border-border-subtle px-4 py-2 last:border-0">
      <Mono className="min-w-0 flex-1 truncate text-xs">{change.id}</Mono>

      {/* The words, never the colour alone. */}
      <Badge variant={regressed ? "destructive" : "outline"}>
        {t(STANDING[change.was])} → {t(STANDING[change.now])}
      </Badge>

      {change.costMicros !== 0 && (
        <Mono dim className="shrink-0 text-2xs">
          {change.costMicros > 0 ? "+" : "−"}
          {formatMicros(Math.abs(change.costMicros))}
        </Mono>
      )}
    </li>
  );
}
