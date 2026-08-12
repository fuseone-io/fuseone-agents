import { useTranslation } from "react-i18next";
import { KpiCard } from "@/components/shared/kpi-card";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDurationMs } from "@/lib/format";
import type { RunStats } from "@/lib/api/client";

const WAITING = ["awaiting_approval", "parked"] as const;

/**
 * Four figures about the whole set, each stated with what it is a share of.
 *
 * "97,2%" alone is a number nobody can act on; "97,2% de 1.284" is one they
 * can. The delta line always carries the basis for that reason.
 */
export function RunsKpis({
  stats,
  isLoading,
}: {
  stats?: RunStats;
  isLoading: boolean;
}) {
  const { t } = useTranslation();
  if (isLoading || !stats) {
    return (
      <div className="flex shrink-0 gap-3">
        {Array.from({ length: 4 }, (_, i) => (
          <Skeleton key={i} className="h-[104px] flex-1 rounded-xl" />
        ))}
      </div>
    );
  }

  const finished = stats.byPhase.finished ?? 0;
  const waiting = WAITING.reduce(
    (sum, phase) => sum + (stats.byPhase[phase] ?? 0),
    0,
  );
  const running = stats.total - finished - waiting;

  return (
    <div className="flex shrink-0 gap-3">
      <KpiCard
        label={t("runs.runs")}
        value={stats.total.toLocaleString("pt-BR")}
        delta={`${running.toLocaleString("pt-BR")} em andamento`}
      />
      <KpiCard
        label={t("runs.finishedPlural")}
        value={stats.total === 0 ? "—" : `${percent(finished, stats.total)}%`}
        delta={`${finished.toLocaleString("pt-BR")} de ${stats.total.toLocaleString("pt-BR")}`}
        trend={stats.total > 0 && finished === stats.total ? "up" : "flat"}
      />
      <KpiCard
        label={t("runs.medianDuration")}
        value={
          stats.ended === 0
            ? "—"
            : formatDurationMs(stats.medianDurationMs ?? 0)
        }
        // The basis is not decoration: a median over three runs and one over
        // three thousand are different claims.
        delta={
          stats.ended === 0
            ? t("runs.noneFinishedYet")
            : `sobre ${stats.ended} ${stats.ended === 1 ? t("runs.finished") : t("overview.doneLegend")}`
        }
      />
      <KpiCard
        label={t("runs.waitingPerson")}
        value={waiting.toLocaleString("pt-BR")}
        delta={
          waiting === 0 ? t("runs.nothingQueued") : t("runs.approvalOrParked")
        }
        trend={waiting > 0 ? "down" : "flat"}
      />
    </div>
  );
}

/**
 * A decimal only where the sample supports one.
 *
 * "33,3% de 3" states a precision three runs cannot carry, and in a monospaced
 * face the decimal comma takes a full cell and reads as a gap. Below a
 * thousand runs one run moves the figure by more than a tenth of a point, so
 * the tenth is noise.
 */
function percent(part: number, whole: number): string {
  const value = (part / whole) * 100;
  if (whole < 1000 || value === 100) return String(Math.round(value));
  return value.toFixed(1).replace(".", ",");
}
