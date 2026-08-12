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
        label="Gasto no período"
        value={formatMicros(total.micros)}
        delta={`${runs.toLocaleString("pt-BR")} ${runs === 1 ? "execução" : "execuções"}`}
      />
      <KpiCard
        label="Hoje"
        value={today ? formatMicros(today.cost.micros) : "—"}
        delta={
          today
            ? `${today.runs} ${today.runs === 1 ? "execução" : "execuções"}`
            : "nada hoje"
        }
      />
      <KpiCard
        label="Custo médio"
        value={runs === 0 ? "—" : formatMicros(Math.round(total.micros / runs))}
        // A mean is the right figure here, unlike for duration: cost per run
        // is what multiplies by volume when somebody asks to scale an agent.
        delta={runs === 0 ? "nenhuma execução ainda" : "por execução"}
      />
      <KpiCard
        label="Leitura de cache"
        value={cacheShare === null ? "—" : `${cacheShare}%`}
        delta={
          cacheShare === null
            ? "sem tokens contabilizados"
            : "do que entrou no modelo, a fração do preço"
        }
        trend={cacheShare !== null && cacheShare >= 50 ? "up" : "flat"}
      />
    </div>
  );
}
