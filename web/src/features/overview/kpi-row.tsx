import { Skeleton } from "@/components/ui/skeleton";
import { useRunStats } from "@/features/runs/api";
import { deltaOf, type Windows } from "@/features/overview/window";
import { OverviewKpi } from "@/features/overview/overview-kpi";
import { formatDurationMs } from "@/lib/format";

/**
 * The four figures the day is judged by.
 *
 * Each carries what it is measured against — yesterday, the runs it was
 * computed over, how many need a person. A bare number invites the reader to
 * invent the comparison, and they will invent a flattering one.
 */
export function KpiRow({ windows }: { windows: Windows }) {
  const today = useRunStats({ since: windows.current.since, until: windows.current.until });
  const yesterday = useRunStats({ since: windows.previous.since, until: windows.previous.until });

  if (today.isLoading) {
    return (
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }, (_, i) => (
          <Skeleton key={i} className="h-[104px] rounded-xl" />
        ))}
      </div>
    );
  }

  const stats = today.data;
  const blocked = stats?.byPhase?.parked ?? 0;
  const waiting = stats?.byPhase?.awaiting_approval ?? 0;
  const before = yesterday.data?.total ?? 0;
  const change = deltaOf(stats?.total ?? 0, before);

  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <OverviewKpi
        label="Execuções hoje"
        value={String(stats?.total ?? 0)}
        delta={change}
        // "vs ontem" beside nothing invites the reader to supply the missing
        // number, and the honest answer is that yesterday had none.
        note={change === undefined ? "nada ontem para comparar" : `vs ${before} ontem`}
      />
      <OverviewKpi
        label="Duração mediana"
        value={stats?.medianDurationMs ? formatDurationMs(stats.medianDurationMs) : "—"}
        note={`sobre ${stats?.ended ?? 0} concluídas`}
      />
      <OverviewKpi
        label="Cauda lenta (p95)"
        value={stats?.p95DurationMs ? formatDurationMs(stats.p95DurationMs) : "—"}
        note="as execuções de que se reclama"
      />
      <OverviewKpi
        label="Paradas"
        value={String(blocked)}
        note={waiting > 0 ? `${waiting} esperando uma pessoa` : "nenhuma esperando decisão"}
        tone={blocked > 0 ? "bad" : "neutral"}
      />
    </div>
  );
}
