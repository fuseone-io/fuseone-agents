import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Mono } from "@/components/shared/mono";
import { TraceStep } from "@/features/overview/trace-step";
import { TraceActions } from "@/features/overview/trace-actions";
import { TraceDecision } from "@/features/overview/trace-decision";
import { useRun, useRunSteps } from "@/features/runs/api";
import { formatCost, formatDuration } from "@/lib/format";
import { cn } from "@/lib/utils";

/**
 * One run's trail, docked beside the overview.
 *
 * The point is not to reproduce the run screen in a narrow column. It is that
 * somebody scanning what happened today can open a run without losing the page
 * they were reading, and decide on it there — a pending approval is answerable
 * from here, because the alternative is that noticing something and acting on
 * it are two different sittings.
 */
export function TracePanel({
  runId,
  onClose,
}: {
  runId: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const run = useRun(runId);
  const steps = useRunSteps(runId);

  const items = steps.data?.items ?? [];
  const pending = run.data?.pendingApproval;

  return (
    // A column, not a card that grows: the steps scroll and the decision stays
    // pinned. A run with forty steps must not push its own Approve button off
    // the screen.
    //
    // Full height by stretching, not by a viewport calculation: the row already
    // measures the space left under the title, and a calc would have to restate
    // the header, the padding and the title block and go wrong the day one of
    // them changes.
    <aside className="flex max-h-[calc(100vh-100px)] w-full flex-col rounded-xl border border-border bg-card shadow-sm lg:w-[340px] lg:shrink-0">
      <header className="flex items-start gap-2 border-b border-border p-4">
        <div className="min-w-0 flex-1">
          <Link
            to={`/runs/${runId}`}
            className="text-sm font-medium hover:underline"
          >
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
        <Button
          variant="ghost"
          size="icon"
          className="size-7 shrink-0"
          onClick={onClose}
        >
          <X className="size-4" aria-hidden />
          <span className="sr-only">{t("overview.closeTrace")}</span>
        </Button>
      </header>

      {run.isLoading || steps.isLoading ? (
        <div className="p-4">
          <Skeleton className="h-40 rounded-lg" />
        </div>
      ) : (
        <>
          <div className="min-h-0 flex-1 overflow-auto p-4">
            {pending && <TraceDecision runId={runId} approval={pending} />}

            <ol className={cn("flex flex-col", pending && "mt-3")}>
              {items.map((step, i) => (
                <TraceStep
                  key={step.seq}
                  step={step}
                  last={i === items.length - 1}
                />
              ))}
            </ol>
          </div>

          {/* Pinned. Answerable here, because noticing something and acting on
              it should not be two different sittings — and an action that
              scrolls away is one nobody reaches. */}
          {pending && (
            <div className="border-t border-border p-3">
              <TraceActions runId={runId} approval={pending} />
            </div>
          )}
        </>
      )}
    </aside>
  );
}
