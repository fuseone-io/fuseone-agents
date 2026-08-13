import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { TrailEvent } from "@/features/runs/trail-event";
import { TrailFold } from "@/features/runs/trail-fold";
import { formatTime } from "@/lib/format";
import type { TrailGroup, TrailPhase } from "@/features/runs/trail-model";

const PHASE_LABEL: Record<TrailPhase, string> = {
  input: "runs.phaseInput",
  execution: "runs.phaseExecution",
  human: "runs.phaseHuman",
  end: "runs.phaseEnd",
};

/** The run as a sequence, grouped by phase and folded where nothing needed a
 *  person, so the eye lands on the decisions. */
export function TrailList({
  runId,
  groups,
  lastSeq,
  liveSeq,
  showHashes,
}: {
  runId: string;
  groups: TrailGroup[];
  lastSeq?: number;
  liveSeq?: number;
  showHashes: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="px-4 pb-4">
      {groups.map((group) => (
        <div key={`${group.phase}-${group.at}`}>
          <div className="flex items-center gap-2.5 py-2 pt-3.5">
            <span className="text-2xs uppercase tracking-label text-muted-foreground">
              {t(PHASE_LABEL[group.phase])}
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
                  key={`fold-${entry.steps[0]?.seq}`}
                  steps={entry.steps}
                  last={entry.steps.at(-1)?.seq === lastSeq}
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
  );
}
