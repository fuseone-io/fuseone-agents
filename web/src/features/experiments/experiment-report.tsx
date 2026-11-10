import { useTranslation } from "react-i18next";
import type { UseQueryResult } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Mono } from "@/components/shared/mono";
import { Panel } from "@/components/shared/panel";
import { ExperimentCaseRow } from "@/features/experiments/experiment-case-row";
import { ExperimentSummaryFacts } from "@/features/experiments/experiment-summary";
import { LoadFailed } from "@/features/experiments/load-failed";
import type { ExperimentReport } from "@/features/experiments/experiments-api";
import {
  Table,
  TableBody,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

/**
 * One experiment: what it ran, then each case across its repetitions.
 *
 * Readable while it is still going — the cases that settled are complete rows —
 * and re-read until nothing is left running.
 */
export function ExperimentReportView({ report }: { report: UseQueryResult<ExperimentReport> }) {
  const { t } = useTranslation();

  if (report.isLoading) {
    return <Skeleton className="h-64 w-full rounded-xl" />;
  }
  if (report.error || !report.data) {
    return (
      <LoadFailed
        message={t("experiments.reportFailed")}
        onRetry={() => void report.refetch()}
      />
    );
  }
  const { summary, cases } = report.data;

  return (
    <div className="flex flex-col gap-5">
      <Panel
        title={<Mono className="text-sm">{summary.id}</Mono>}
        action={
          summary.running ? (
            <Badge variant="outline">{t("experiments.stillRunning")}</Badge>
          ) : undefined
        }
      >
        <ExperimentSummaryFacts summary={summary} />
      </Panel>

      <Panel title={t("experiments.casesTitle")} flush>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("experiments.case")}</TableHead>
              <TableHead>{t("experiments.standing")}</TableHead>
              <TableHead>{t("experiments.brokenInColumn")}</TableHead>
              <TableHead className="text-right">{t("experiments.cost")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {cases.map((c, at) => (
              <ExperimentCaseRow
                key={c.case.id ?? `case-${at}`}
                row={c}
                repetitions={summary.repetitions}
              />
            ))}
          </TableBody>
        </Table>
      </Panel>
    </div>
  );
}
