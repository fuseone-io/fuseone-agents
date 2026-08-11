import { Link } from "react-router-dom";
import { Skeleton } from "@/components/ui/skeleton";
import { Mono } from "@/components/shared/mono";
import { StateDot } from "@/components/shared/state-dot";
import { useRuns } from "@/features/runs/api";
import { stateOfPhase } from "@/lib/agent-state";
import { PHASE_LABELS } from "@/features/runs/phase-badge";
import { formatCost, formatTime } from "@/lib/format";

/**
 * The last few runs, as a way into them.
 *
 * Deliberately short and deliberately not the Runs table: this is the panel
 * somebody clicks through from, not the one they work in. Filtering and
 * pagination belong on the screen built for them.
 */
export function RecentRuns({ since }: { since: string }) {
  const { data, isLoading, error } = useRuns({ since });
  const runs = (data?.items ?? []).slice(0, 8);

  return (
    <section className="flex flex-col gap-2 rounded-xl border border-border bg-card p-4 shadow-sm">
      <div className="flex items-center gap-2">
        <h2 className="text-sm font-medium">Execuções recentes</h2>
        <Link to="/runs" className="ml-auto text-xs text-primary hover:underline">
          ver todas
        </Link>
      </div>

      {isLoading ? (
        <div className="flex flex-col gap-2 py-1">
          {Array.from({ length: 5 }, (_, i) => (
            <Skeleton key={i} className="h-8 rounded" />
          ))}
        </div>
      ) : error ? (
        <p className="py-8 text-center text-sm text-muted-foreground">
          Não foi possível ler as execuções.
        </p>
      ) : runs.length === 0 ? (
        <p className="py-8 text-center text-sm text-muted-foreground">
          Nenhuma execução hoje. Agentes disparam por agendamento, webhook ou evento.
        </p>
      ) : (
        <ol className="flex flex-col">
          {runs.map((run) => (
            <li key={run.runId}>
              <Link
                to={`/runs/${run.runId}`}
                className="flex h-8 items-center gap-2 rounded-md px-1 hover:bg-muted"
              >
                <StateDot state={stateOfPhase(run.phase)} />
                <span className="truncate text-xs">{run.agentId}</span>
                <span className="shrink-0 text-xs text-muted-foreground">
                  {PHASE_LABELS[run.phase]}
                </span>
                <Mono dim className="ml-auto shrink-0 text-2xs">
                  {formatCost(run.cost)} · {formatTime(run.startedAt)}
                </Mono>
              </Link>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
