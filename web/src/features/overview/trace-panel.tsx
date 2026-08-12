import { Link } from "react-router-dom";
import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Mono } from "@/components/shared/mono";
import { TraceStep } from "@/features/overview/trace-step";
import { PendingDecision } from "@/features/runs/pending-decision";
import { useRun, useRunSteps } from "@/features/runs/api";
import { formatCost, formatDuration } from "@/lib/format";

/**
 * One run's trail, docked beside the overview.
 *
 * The point is not to reproduce the run screen in a narrow column. It is that
 * somebody scanning what happened today can open a run without losing the page
 * they were reading, and decide on it there — a pending approval is answerable
 * from here, because the alternative is that noticing something and acting on
 * it are two different sittings.
 */
export function TracePanel({ runId, onClose }: { runId: string; onClose: () => void }) {
  const run = useRun(runId);
  const steps = useRunSteps(runId);

  const items = steps.data?.items ?? [];
  const pending = run.data?.pendingApproval;

  return (
    <aside className="flex w-full flex-col gap-3 rounded-xl border border-border bg-card p-4 shadow-sm lg:sticky lg:top-0 lg:w-[340px] lg:shrink-0">
      <header className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <Link to={`/runs/${runId}`} className="text-sm font-medium hover:underline">
            {run.data?.agentId ?? runId}
          </Link>
          <div className="truncate">
            <Mono dim className="text-2xs">
              {runId}
              {run.data
                ? ` · ${items.length} passos · ${formatCost(run.data.cost)} · ${formatDuration(run.data.startedAt, run.data.endedAt)}`
                : ""}
            </Mono>
          </div>
        </div>
        <Button variant="ghost" size="icon" className="size-7 shrink-0" onClick={onClose}>
          <X className="size-4" aria-hidden />
          <span className="sr-only">Fechar o rastro</span>
        </Button>
      </header>

      {run.isLoading || steps.isLoading ? (
        <Skeleton className="h-40 rounded-lg" />
      ) : (
        <>
          {/* Answerable here. Noticing something and acting on it should not
              be two different sittings. */}
          {pending && (
            <PendingDecision
              runId={runId}
              approval={pending}
              step={items.find((s) => s.seq === pending.atSeq)}
            />
          )}

          <ol className="flex flex-col">
            {items.map((step, i) => (
              <TraceStep key={step.seq} step={step} last={i === items.length - 1} />
            ))}
          </ol>
        </>
      )}
    </aside>
  );
}
