import { useTranslation } from "react-i18next";
import { Skeleton } from "@/components/ui/skeleton";
import { LoadFailed } from "@/features/experiments/load-failed";
import { useExperimentComparison, type ExperimentReport } from "@/features/experiments/experiments-api";
import { CaseRunGroup } from "@/features/workbench/case-run-group";
import { GROUPS, grouped } from "@/features/workbench/case-run-groups";

/**
 * The candidate against its baseline, case by case, in the groups NT-018 names. There is no
 * judge column: it waits for NT-016's second release, and an empty one would read as a
 * judge that found nothing.
 */
export function CaseRunComparison({
  agentId,
  baseline,
  candidate,
  onTrace,
}: {
  agentId: string;
  baseline: string;
  candidate: ExperimentReport;
  onTrace: (runId: string) => void;
}) {
  const { t } = useTranslation();
  const comparison = useExperimentComparison(agentId, baseline, candidate.summary.id);

  if (comparison.isLoading) return <Skeleton className="h-48 w-full rounded-xl" />;
  if (comparison.error || !comparison.data) {
    return <LoadFailed message={t("experiments.comparisonFailed")} onRetry={() => void comparison.refetch()} />;
  }
  const groups = grouped(comparison.data.cases);
  // The run a regression opens is the candidate's own, named by its case.
  const runs = new Map(candidate.cases.map((c) => [c.case.id ?? "", c.case.runId]));

  return (
    <div className="flex flex-col gap-3">
      {comparison.data.sharedOnly && (
        <p className="text-xs text-muted-foreground">
          {t("workbench.experiment.sharedOnly", { onlyFrom: comparison.data.onlyFrom, onlyTo: comparison.data.onlyTo })}
        </p>
      )}
      <div className="grid gap-3 lg:grid-cols-2">
        {GROUPS.map((group) => (
          <CaseRunGroup key={group} group={group} changes={groups[group]} runOf={(id) => runs.get(id)} onTrace={onTrace} />
        ))}
      </div>
    </div>
  );
}
