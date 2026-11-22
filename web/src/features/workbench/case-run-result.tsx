import { useTranslation } from "react-i18next";
import { Info, RotateCcw } from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Mono } from "@/components/shared/mono";
import { ExperimentReportView } from "@/features/experiments/experiment-report";
import { LoadFailed } from "@/features/experiments/load-failed";
import { useExperiment } from "@/features/experiments/experiments-api";
import { CaseRunComparison } from "@/features/workbench/case-run-comparison";

type Opened = { experiment: string; baseline: string };

/**
 * One run against the cases, as the address names it: the candidate's experiment and the
 * baseline it is compared with. The comparison waits until both have ended, so it never
 * counts a case one side has not settled yet. Without a baseline, the agent was never
 * published, and the candidate's report stands alone.
 */
export function CaseRunResult({ agentId, opened, onTrace, onNew }: { agentId: string; opened: Opened; onTrace: (runId: string) => void; onNew: () => void }) {
  const { t } = useTranslation();
  const candidate = useExperiment(agentId, opened.experiment);
  const baseline = useExperiment(agentId, opened.baseline);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
        <span>{t("workbench.experiment.candidateExperiment")} <Mono>{opened.experiment}</Mono></span>
        {opened.baseline && <span>{t("workbench.experiment.baseline")} <Mono>{opened.baseline}</Mono></span>}
        <Button size="sm" variant="outline" className="ml-auto" onClick={onNew}>
          <RotateCcw className="size-4" aria-hidden />
          {t("workbench.experiment.again")}
        </Button>
      </div>
      {opened.baseline ? <Compared agentId={agentId} opened={opened} reads={{ candidate, baseline }} onTrace={onTrace} /> : <Alone report={candidate} />}
    </div>
  );
}

function Alone({ report }: { report: ReturnType<typeof useExperiment> }) {
  const { t } = useTranslation();
  return (
    <>
      <Alert>
        <Info className="size-4" aria-hidden />
        <AlertDescription>{t("workbench.experiment.noBaseline")}</AlertDescription>
      </Alert>
      <ExperimentReportView report={report} />
    </>
  );
}

function Compared({
  agentId,
  opened,
  reads,
  onTrace,
}: {
  agentId: string;
  opened: Opened;
  reads: { candidate: ReturnType<typeof useExperiment>; baseline: ReturnType<typeof useExperiment> };
  onTrace: (runId: string) => void;
}) {
  const { t } = useTranslation();
  const { candidate, baseline } = reads;
  if (candidate.isLoading || baseline.isLoading) {
    return <Skeleton aria-label={t("workbench.experiment.reading")} className="h-48 w-full rounded-xl" />;
  }
  const failed = candidate.error ? candidate : baseline.error ? baseline : undefined;
  if (failed || !candidate.data || !baseline.data) {
    return <LoadFailed message={t("experiments.reportFailed")} onRetry={() => void (failed ?? candidate).refetch()} />;
  }
  if (candidate.data.summary.running || baseline.data.summary.running) {
    return (
      <div role="status" className="flex flex-wrap items-center gap-2 rounded-lg border border-dashed p-4 text-sm">
        <Badge variant="outline">{t("experiments.stillRunning")}</Badge>
        <span className="text-muted-foreground">{t("workbench.experiment.waiting")}</span>
      </div>
    );
  }
  return <CaseRunComparison agentId={agentId} baseline={opened.baseline} candidate={candidate.data} onTrace={onTrace} />;
}
