import { useTranslation } from "react-i18next";
import { CircleDollarSign } from "lucide-react";
import { useRegressions } from "@/features/agents/regressions-api";
import { formatMicros } from "@/lib/format";
import type { Agent } from "@/lib/api/client";

/**
 * What the experiment may spend, stated before it starts (NT-015).
 *
 * Every repetition is its own run, reserved and billed, so the preview counts
 * runs and not cases: five repetitions of forty cases is two hundred runs, each
 * up to the version's ceiling.
 */
export function ExperimentCost({
  agentId,
  agent,
  repetitions,
}: {
  agentId: string;
  agent?: Agent;
  repetitions: number;
}) {
  const { t } = useTranslation();
  const corpus = useRegressions(agentId);
  const cases = corpus.data?.items.length;
  const perRun = agent?.budget.micros ?? 0;

  return (
    <div className="flex min-w-0 items-start gap-3 rounded-lg border bg-muted p-4">
      <CircleDollarSign className="mt-0.5 size-4 shrink-0 text-text-accent" aria-hidden />
      <div className="min-w-0 space-y-1">
        <p className="text-sm font-medium">{t("experiments.costTitle")}</p>
        <p className="text-sm text-muted-foreground">
          {costText({ t, cases, repetitions, perRun, error: corpus.isError })}
        </p>
      </div>
    </div>
  );
}

function costText({
  t,
  cases,
  repetitions,
  perRun,
  error,
}: {
  t: ReturnType<typeof useTranslation>["t"];
  cases?: number;
  repetitions: number;
  perRun: number;
  error: boolean;
}) {
  if (error) return t("experiments.costUnknown");
  if (cases === undefined) return t("experiments.costCounting");
  const runs = cases * repetitions;
  if (perRun <= 0) return t("experiments.costUnbounded", { count: runs, cases, repetitions });
  return t("experiments.costBounded", {
    count: runs,
    cases,
    repetitions,
    perRun: formatMicros(perRun),
    total: formatMicros(perRun * runs),
  });
}
