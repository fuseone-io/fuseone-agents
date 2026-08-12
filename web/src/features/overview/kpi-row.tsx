import { useTranslation } from "react-i18next";
import { Skeleton } from "@/components/ui/skeleton";
import { useRunStats } from "@/features/runs/api";
import { useThroughput } from "@/features/overview/api";
import { deltaOf, type Windows } from "@/features/overview/window";
import { OverviewKpi } from "@/features/overview/overview-kpi";
import { columnsFor } from "@/features/overview/throughput-model";
import { formatCost, formatDurationMs } from "@/lib/format";

/**
 * The four figures the day is judged by: how much ran, what it cost, how slow
 * the slow end was, and what is stuck.
 *
 * Each carries what it is measured against. A bare number invites the reader
 * to invent the comparison, and they will invent a flattering one.
 */
export function KpiRow({ windows }: { windows: Windows }) {
  const { t } = useTranslation();
  const today = useRunStats({
    since: windows.current.since,
    until: windows.current.until,
  });
  const yesterday = useRunStats({
    since: windows.previous.since,
    until: windows.previous.until,
  });
  const hours = useThroughput(windows.current.since);

  if (today.isLoading) {
    return (
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }, (_, i) => (
          <Skeleton key={i} className="h-[120px] rounded-xl" />
        ))}
      </div>
    );
  }

  const stats = today.data;
  const before = yesterday.data;
  const columns = columnsFor(hours.data?.buckets ?? [], windows.current.since);

  const runs = stats?.total ?? 0;
  const spent = columns.reduce((sum, c) => sum + c.micros, 0);
  const blocked = stats?.byPhase?.parked ?? 0;
  const waiting = stats?.byPhase?.awaiting_approval ?? 0;
  const runsDelta = deltaOf(runs, before?.total ?? 0);

  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <OverviewKpi
        label={t("overview.runsToday")}
        value={String(runs)}
        delta={runsDelta}
        // "vs ontem" beside nothing invites the reader to supply the missing
        // number, and the honest answer is that yesterday had none.
        note={
          runsDelta === undefined
            ? "nada ontem para comparar"
            : `vs ${before?.total ?? 0} ontem`
        }
        trend={columns.map((c) => c.total)}
      />

      <OverviewKpi
        label="Gasto hoje"
        value={formatCost({ micros: spent })}
        note={`sobre ${runs} execuções`}
        trend={cumulative(columns.map((c) => c.micros))}
      />

      <OverviewKpi
        label="Cauda lenta (p95)"
        value={
          stats?.p95DurationMs ? formatDurationMs(stats.p95DurationMs) : "—"
        }
        delta={deltaOf(stats?.p95DurationMs ?? 0, before?.p95DurationMs ?? 0)}
        note={`mediana ${stats?.medianDurationMs ? formatDurationMs(stats.medianDurationMs) : "—"}`}
      />

      <OverviewKpi
        label="Paradas"
        value={String(blocked)}
        note={
          waiting > 0
            ? `${waiting} esperando uma pessoa`
            : t("overview.noneWaiting")
        }
        tone={blocked > 0 ? "bad" : "neutral"}
        trend={columns.map((c) => c.byState.blocked)}
      />
    </div>
  );
}

/** Spend is what has accumulated, not what each hour added — the card asks
 *  how the day's bill grew. */
function cumulative(values: number[]): number[] {
  let total = 0;
  return values.map((v) => (total += v));
}
