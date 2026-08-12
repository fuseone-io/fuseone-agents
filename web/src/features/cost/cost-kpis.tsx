import { useTranslation } from "react-i18next";
import { KpiCard } from "@/components/shared/kpi-card";
import { Skeleton } from "@/components/ui/skeleton";
import { formatMicros } from "@/lib/format";
import type { components } from "@/lib/api/schema.gen";

type CostRollup = components["schemas"]["CostRollup"];

/**
 * Four figures about spend, each with the basis it rests on.
 *
 * The design's fourth card is "caps hit". This installation has no ceiling to
 * hit — budgets live inside each agent's specification, not per scope (PRD
 * FO-02 is unbuilt) — so it reports the cache share instead, which is real,
 * actionable, and the lever the platform is responsible for (FO-09).
 */
export function CostKpis({
  daily,
  isLoading,
}: {
  daily?: CostRollup;
  isLoading: boolean;
}) {
  const { t } = useTranslation();
  if (isLoading || !daily) {
    return (
      <div className="flex shrink-0 gap-3">
        {Array.from({ length: 4 }, (_, i) => (
          <Skeleton key={i} className="h-[104px] flex-1 rounded-xl" />
        ))}
      </div>
    );
  }

  const buckets = daily.buckets;
  const runs = buckets.reduce((sum, b) => sum + b.runs, 0);
  const today = buckets.at(-1);
  const total = daily.total;

  const input = total.inputTokens ?? 0;
  const cached = total.cacheReadTokens ?? 0;
  const cacheShare =
    input + cached === 0 ? null : Math.round((cached / (input + cached)) * 100);

  return (
    <div className="flex shrink-0 gap-3">
      <KpiCard
        label={t("cost.spentInPeriod")}
        value={formatMicros(total.micros)}
        delta={t("runs.runCount", { count: runs })}
      />
      <KpiCard
        label={t("cost.today")}
        value={today ? formatMicros(today.cost.micros) : "—"}
        delta={
          today
            ? t("runs.runCount", { count: today.runs })
            : t("overview.nothingToday")
        }
      />
      <KpiCard
        label={t("cost.averageCost")}
        value={runs === 0 ? "—" : formatMicros(Math.round(total.micros / runs))}
        // A mean is the right figure here, unlike for duration: cost per run
        // is what multiplies by volume when somebody asks to scale an agent.
        delta={runs === 0 ? t("cost.noRunsYet") : t("cost.perRun")}
      />
      <KpiCard
        label="Leitura de cache"
        value={cacheShare === null ? "—" : `${cacheShare}%`}
        delta={cacheShare === null ? t("cost.noTokens2") : t("cost.cacheShare")}
        trend={cacheShare !== null && cacheShare >= 50 ? "up" : "flat"}
      />
    </div>
  );
}
