import { useParams } from "react-router-dom";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { PendingDecision } from "@/features/runs/pending-decision";
import { RunIdentity } from "@/features/runs/run-identity";
import { RunKpis } from "@/features/runs/run-kpis";
import { RunFailureNotice } from "@/features/runs/run-failure-notice";
import { RunSideRail } from "@/features/runs/run-side-rail";
import { TrailPanel } from "@/features/runs/trail-panel";
import { useRun, useRunSteps } from "@/features/runs/api";
import type { Step } from "@/lib/api/client";

/**
 * One run, end to end.
 *
 * Reading order is the order the questions arrive: which run is this, does it
 * need me right now, how is it going, and then what happened. The decision
 * card sits above the trail because somebody who has to answer it should not
 * have to scroll past eighteen events to find the question.
 */
export function RunDetailPage() {
  const { runId = "" } = useParams();
  const run = useRun(runId);
  const steps = useRunSteps(runId);

  if (run.isLoading || steps.isLoading) return <LoadingRows rows={8} />;
  if (run.error)
    return <ErrorState error={run.error} onRetry={() => void run.refetch()} />;
  if (steps.error)
    return (
      <ErrorState error={steps.error} onRetry={() => void steps.refetch()} />
    );
  if (!run.data) return null;

  const { data } = run;
  const items = steps.items;
  const pending = data.pendingApproval;

  return (
    <div className="flex w-full min-w-0 flex-col gap-5">
      <RunIdentity run={data} trigger={triggerOf(items)} />
      <RunFailureNotice run={data} />

      {pending && (
        <PendingDecision
          runId={runId}
          approval={pending}
          step={items.find((step) => step.seq === pending.atSeq)}
        />
      )}

      <RunKpis run={data} steps={items.length} />

      <div className="grid gap-5 lg:grid-cols-[1fr_300px] lg:items-start">
        <TrailPanel
          runId={runId}
          steps={items}
          liveSeq={data.endedAt ? undefined : items[items.length - 1]?.seq}
        />
        <RunSideRail run={data} steps={items} />
      </div>
    </div>
  );
}

/** What started the run, read off the first step rather than the run summary:
 *  the ledger is the record, and the projection is derived from it. */
function triggerOf(steps: Step[]): string | undefined {
  const started = steps.find((step) => step.kind === "run_started");
  const trigger = (started?.payload as Record<string, unknown> | undefined)
    ?.trigger;
  return typeof trigger === "string" ? trigger : undefined;
}
