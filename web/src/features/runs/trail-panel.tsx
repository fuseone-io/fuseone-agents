import { useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { Mono } from "@/components/shared/mono";
import { TrailEvent } from "@/features/runs/trail-event";
import { TrailFold } from "@/features/runs/trail-fold";
import { TrailFilters } from "@/features/runs/trail-filters";
import { buildTrail, type TrailFilter, type TrailPhase } from "@/features/runs/trail-model";
import { formatTime } from "@/lib/format";
import type { Step } from "@/lib/api/client";

const PHASE_LABEL: Record<TrailPhase, string> = {
  input: "Entrada",
  execution: "Execução",
  human: "Decisão humana",
  end: "Encerramento",
};

/**
 * The run, as a sequence a person can read.
 *
 * Grouped by phase and folded where nothing needed a person, so the eye lands
 * on the decisions. The filters narrow rather than reorder: the trail is the
 * audit record, and a view that reordered it would not be one.
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
  const [showHashes, setShowHashes] = useState(true);

  const groups = useMemo(() => buildTrail(steps, { filter }), [steps, filter]);
  const lastSeq = steps[steps.length - 1]?.seq;

  return (
    <section className="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
      <div className="flex flex-wrap items-center gap-2 border-b border-border px-4 py-3">
        <h2 className="text-sm font-medium">Trilha</h2>
        <span className="text-xs text-muted-foreground">{steps.length} eventos</span>
        <div className="ml-auto flex items-center gap-1.5">
          <TrailFilters value={filter} onChange={setFilter} />
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
        </div>
      </div>

      <div className="px-4 pb-4">
        {groups.map((group) => (
          <div key={`${group.phase}-${group.at}`}>
            <div className="flex items-center gap-2.5 py-2 pt-3.5">
              <span className="text-2xs uppercase tracking-label text-muted-foreground">
                {PHASE_LABEL[group.phase]}
              </span>
              <span aria-hidden className="h-px flex-1 bg-border-subtle" />
              <Mono dim className="text-2xs">
                {formatTime(group.at)}
              </Mono>
            </div>

            <ol className="flex flex-col">
              {group.entries.map((entry) =>
                entry.kind === "fold" ? (
                  <TrailFold
                    key={`fold-${entry.steps[0].seq}`}
                    steps={entry.steps}
                    last={entry.steps[entry.steps.length - 1].seq === lastSeq}
                  />
                ) : (
                  <TrailEvent
                    key={entry.step.seq}
                    runId={runId}
                    step={entry.step}
                    live={entry.step.seq === liveSeq}
                    last={entry.step.seq === lastSeq}
                    showHashes={showHashes}
                  />
                ),
              )}
            </ol>
          </div>
        ))}
      </div>
    </section>
  );
}
