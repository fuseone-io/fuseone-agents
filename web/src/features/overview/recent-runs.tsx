import { Link } from "react-router-dom";
import { ScrollArea } from "@/components/ui/scroll-area";
import { LoadingRows } from "@/components/shared/states";
import { RunsTable } from "@/features/runs/runs-table";
import { useRuns } from "@/features/runs/api";
import { cn } from "@/lib/utils";

// A 30px head over five 40px rows. The table keeps eight so there is
// something to scroll to; anything past that belongs on the Runs screen.
const VISIBLE = 5;
const FETCHED = 8;

/**
 * The last few runs, in the same table the Runs screen uses.
 *
 * The same table on purpose: a reader who learns the columns here should not
 * have to learn them again one click later. Filtering and pagination stay on
 * the screen built for them — this is the panel somebody clicks through from,
 * not the one they work in.
 */
export function RecentRuns({
  since,
  selected,
  onSelect,
}: {
  since: string;
  selected?: string;
  onSelect?: (runId: string) => void;
}) {
  const { data, isLoading, error } = useRuns({ since });
  const runs = (data?.items ?? []).slice(0, FETCHED);

  return (
    <section className="flex flex-col gap-2.5">
      <div className="flex items-center gap-2">
        <h2 className="text-sm font-medium">Execuções recentes</h2>
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
            Não foi possível ler as execuções.
          </p>
        ) : runs.length === 0 ? (
          <p className="p-8 text-center text-sm text-muted-foreground">
            Nenhuma execução hoje. Agentes disparam por agendamento, webhook ou evento.
          </p>
        ) : (
          <ScrollArea
            type="auto"
            className={cn(runs.length > VISIBLE && "h-[230px]")}
          >
            <RunsTable runs={runs} selected={selected} onSelect={onSelect} />
          </ScrollArea>
        )}
      </div>
    </section>
  );
}
