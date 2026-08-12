import { useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { RunDiagram } from "@/features/runs/run-diagram";
import { TrailFilters } from "@/features/runs/trail-filters";
import { TrailList } from "@/features/runs/trail-list";
import { TrailViewToggle, type TrailView } from "@/features/runs/trail-view";
import { buildTrail, keptSteps, type TrailFilter } from "@/features/runs/trail-model";
import type { Step } from "@/lib/api/client";

/**
 * The run, read either way.
 *
 * The list is the audit record and the diagram is the same steps under the
 * same filter — neither summarises the other. The filters narrow rather than
 * reorder: a view that reordered the trail would not be an audit record.
 */
export function TrailPanel({
  runId,
  steps,
  liveSeq,
}: {
  runId: string;
  steps: Step[];
  liveSeq?: number;
}) {
  const [filter, setFilter] = useState<TrailFilter>("all");
  const [view, setView] = useState<TrailView>("list");
  const [showHashes, setShowHashes] = useState(true);

  const openStep = (seq: number) => {
    setView("list");
    // After the list has rendered. A fold may hold the step, in which case the
    // panel still lands on the right stretch of the run.
    requestAnimationFrame(() =>
      document.getElementById(`step-${seq}`)?.scrollIntoView({ block: "center" }),
    );
  };

  const groups = useMemo(() => buildTrail(steps, { filter }), [steps, filter]);
  const drawn = useMemo(() => keptSteps(steps, filter), [steps, filter]);
  const lastSeq = steps[steps.length - 1]?.seq;

  return (
    <section className="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
      <div className="flex flex-wrap items-center gap-2 border-b border-border px-4 py-3">
        <h2 className="text-sm font-medium">Trilha</h2>
        <span className="text-xs text-muted-foreground">{steps.length} eventos</span>
        <div className="ml-auto flex items-center gap-1.5">
          <TrailViewToggle value={view} onChange={setView} />
          <Separator orientation="vertical" className="mx-0.5 !h-4" />
          <TrailFilters value={filter} onChange={setFilter} />
          {/* Only where there are seals to show. The diagram draws acts, not
              records, and a toggle that changed nothing would be a dead
              control on a screen about trustworthiness. */}
          {view === "list" && (
            <>
              <Separator orientation="vertical" className="mx-0.5 !h-4" />
              <Button
                variant="outline"
                size="sm"
                className="h-[26px] px-2.5 text-xs font-normal text-muted-foreground"
                aria-pressed={showHashes}
                onClick={() => setShowHashes((on) => !on)}
              >
                {showHashes ? "Ocultar selos" : "Mostrar selos"}
              </Button>
            </>
          )}
        </div>
      </div>

      {view === "list" ? (
        <TrailList
          runId={runId}
          groups={groups}
          lastSeq={lastSeq}
          liveSeq={liveSeq}
          showHashes={showHashes}
        />
      ) : (
        // Clicking a node opens the record it stands for. The diagram is where
        // somebody notices which step to look at; leaving them to find it
        // again by eye in the list would waste the noticing.
        <RunDiagram steps={drawn} onSelect={openStep} />
      )}
    </section>
  );
}
