import { Link } from "react-router-dom";
import { LoadingRows } from "@/components/shared/states";
import { RunsTable } from "@/features/runs/runs-table";
import { useRuns } from "@/features/runs/api";

/**
 * What this agent actually did.
 *
 * Every version's runs, not just this one's: the page is opened to find out
 * whether an agent is behaving, and hiding the runs of the version somebody is
 * about to replace would answer the wrong question.
 */
export function AgentRuns({ agentId }: { agentId: string }) {
  const { data, isLoading, error } = useRuns({ agentId });
  const runs = data?.items ?? [];

  return (
    <section className="flex flex-col gap-2.5">
      <div className="flex items-center gap-2">
        <h2 className="text-sm font-medium">Execuções</h2>
        <span className="text-xs text-muted-foreground">{runs.length}</span>
        <Link to="/runs" className="ml-auto text-xs text-primary hover:underline">
          ver todas
        </Link>
      </div>

      <div className="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
        {isLoading ? (
          <div className="p-4">
            <LoadingRows rows={5} />
          </div>
        ) : error ? (
          <p className="p-8 text-center text-sm text-muted-foreground">
            Não foi possível ler as execuções deste agente.
          </p>
        ) : runs.length === 0 ? (
          <p className="p-8 text-center text-sm text-muted-foreground">
            Este agente ainda não executou. Execuções começam pela linha de
            comando enquanto os gatilhos não existem.
          </p>
        ) : (
          <RunsTable runs={runs} />
        )}
      </div>
    </section>
  );
}
