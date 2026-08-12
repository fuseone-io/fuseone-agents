import { costPerRun, successRate } from "@/features/agents/activity";
import { formatCost, formatRelative } from "@/lib/format";
import type { Agent } from "@/lib/api/client";

/**
 * How the agent has been doing, over every run it has ever had.
 *
 * All time rather than today: an agent that has not run since Tuesday is
 * exactly what somebody opening this page wants to find out, and a window
 * would report it as an empty screen.
 */
export function AgentKpis({ agent }: { agent: Agent }) {
  const activity = agent.activity;
  const perRun = costPerRun(agent);

  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <Kpi
        label="Execuções"
        value={String(activity?.runs ?? 0)}
        note="desde a primeira versão"
      />
      <Kpi
        label="Concluídas"
        value={successRate(agent)}
        note={`${activity?.finished ?? 0} de ${activity?.runs ?? 0}`}
      />
      <Kpi
        label="Custo por execução"
        value={perRun === undefined ? "—" : formatCost({ micros: perRun })}
        note={`${formatCost({ micros: activity?.costMicros ?? 0 })} no total`}
      />
      <Kpi
        label="Esperando pessoas"
        value={String(activity?.waiting ?? 0)}
        note={
          activity?.lastRunAt
            ? `última execução ${formatRelative(activity.lastRunAt)}`
            : "nunca executou"
        }
      />
    </div>
  );
}

function Kpi({
  label,
  value,
  note,
}: {
  label: string;
  value: string;
  note: string;
}) {
  return (
    <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
      <div className="text-2xs uppercase tracking-label text-muted-foreground">
        {label}
      </div>
      <div className="mt-1.5 font-mono text-[22px]/7 font-medium tabular-nums">
        {value}
      </div>
      <div className="mt-0.5 text-xs text-muted-foreground">{note}</div>
    </div>
  );
}
