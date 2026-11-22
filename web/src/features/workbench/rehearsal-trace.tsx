import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Eye, X } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { LoadFailed } from "@/features/experiments/load-failed";
import { PhaseBadge } from "@/features/runs/phase-badge";
import { TrailPanel } from "@/features/runs/trail-panel";
import { useRehearsal } from "@/features/workbench/rehearsal-api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * A run read in the rehearsal tab and nothing more: an experiment's run, opened from a
 * regression. It offers nothing to keep, answer, decide or abandon, whatever the run is,
 * because none of those belongs to a run nobody rehearsed. A run that cannot be read says
 * so here, and the tab does not fall back to a rehearsal.
 */
export function RehearsalTrace({ runId, onClose }: { runId: string; onClose: () => void }) {
  const { t } = useTranslation();
  const { run, steps } = useRehearsal(runId);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <Eye className="size-4 text-muted-foreground" aria-hidden />
        <p className="text-sm text-muted-foreground">{t("workbench.trace.readOnly")}</p>
        <Button size="sm" variant="outline" className="ml-auto" onClick={onClose}>
          <X className="size-4" aria-hidden />
          {t("workbench.trace.close")}
        </Button>
      </div>
      {run.isLoading && <Skeleton aria-label={t("workbench.trace.reading")} className="h-40 w-full" />}
      {run.error && <LoadFailed message={problemMessage(run.error, t)} onRetry={() => void run.refetch()} />}
      {run.data && (
        <div className="flex flex-wrap items-center gap-2">
          <PhaseBadge phase={run.data.phase} />
          <Button asChild size="sm" variant="link" className="h-auto p-0">
            <Link to={`/runs/${runId}`}><Mono>{runId}</Mono></Link>
          </Button>
        </div>
      )}
      {steps.error && <LoadFailed message={problemMessage(steps.error, t)} onRetry={() => void steps.refetch()} />}
      {run.data && (steps.data?.length ?? 0) > 0 && <TrailPanel runId={runId} steps={steps.data ?? []} />}
    </div>
  );
}
