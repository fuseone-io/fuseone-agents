import { useTranslation } from "react-i18next";
import { Info } from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { Panel } from "@/components/shared/panel";
import { ExperimentChangeRow } from "@/features/experiments/experiment-change-row";
import { ExperimentSummaryFacts } from "@/features/experiments/experiment-summary";
import { LoadFailed } from "@/features/experiments/load-failed";
import { useExperimentComparison } from "@/features/experiments/experiments-api";
import { formatMicros } from "@/lib/format";
import type { ExperimentComparison } from "@/features/experiments/experiments-api";
import {
  Table,
  TableBody,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

/**
 * Two experiments of one agent, case by case (NT-015 § Experiments).
 *
 * On different dataset versions only the cases both had are compared, and the
 * screen says so before anything else: a comparison that quietly dropped the
 * cases one side never had would read as covering them.
 */
export function ExperimentComparisonView({
  agentId,
  from,
  to,
}: {
  agentId: string;
  from: string;
  to: string;
}) {
  const { t } = useTranslation();
  const comparison = useExperimentComparison(agentId, from, to);

  if (comparison.isLoading) return <Skeleton className="h-64 w-full rounded-xl" />;
  if (comparison.error || !comparison.data) {
    return (
      <LoadFailed
        message={t("experiments.comparisonFailed")}
        onRetry={() => void comparison.refetch()}
      />
    );
  }
  const data = comparison.data;

  return (
    <div className="flex flex-col gap-5">
      {data.sharedOnly && <SharedOnlyNotice data={data} />}
      <div className="grid gap-5 lg:grid-cols-2">
        <Panel title={t("experiments.from")}>
          <ExperimentSummaryFacts summary={data.from} />
        </Panel>
        <Panel title={t("experiments.to")}>
          <ExperimentSummaryFacts summary={data.to} />
        </Panel>
      </div>
      <Panel title={t("experiments.changesTitle")} flush>
        <p className="flex flex-wrap gap-x-4 gap-y-1 px-4 pb-3 text-sm">
          <span>{t("comparison.regressed", { count: data.regressed })}</span>
          <span>{t("comparison.fixed", { count: data.fixed })}</span>
          <span className="text-muted-foreground">
            {t(data.costMicros < 0 ? "experiments.costPerRunDown" : "experiments.costPerRunUp", {
              amount: formatMicros(Math.abs(data.costMicros)),
            })}
          </span>
        </p>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("experiments.case")}</TableHead>
              <TableHead>{t("experiments.standing")}</TableHead>
              <TableHead>{t("experiments.brokenInColumn")}</TableHead>
              <TableHead className="text-right">{t("experiments.costPerRun")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.cases.map((change) => (
              <ExperimentChangeRow key={change.id} change={change} />
            ))}
          </TableBody>
        </Table>
      </Panel>
    </div>
  );
}

function SharedOnlyNotice({ data }: { data: ExperimentComparison }) {
  const { t } = useTranslation();
  return (
    <Alert>
      <Info className="size-4" aria-hidden />
      <AlertDescription>
        {t("experiments.sharedOnly", {
          from: data.from.datasetVersion,
          to: data.to.datasetVersion,
          count: data.cases.length,
          onlyFrom: data.onlyFrom,
          onlyTo: data.onlyTo,
        })}
      </AlertDescription>
    </Alert>
  );
}
