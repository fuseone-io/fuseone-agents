import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { SimulationSummary } from "@/features/agents/simulation-summary";
import { CaseRow } from "@/features/agents/simulation-case";
import { CorrectionDialog } from "@/features/agents/correction-dialog";
import { VersionComparison } from "@/features/agents/version-comparison";
import type { SimulationCase } from "@/features/agents/simulation-api";
import type { useSimulation } from "@/features/agents/simulation-api";

/**
 * The four states a report can be in, and the rows once it has them.
 *
 * A partial report is not a partial answer: the cases that have settled are
 * complete rows and the rest say they have not, because the whole thing is a
 * fold of runs still being advanced.
 */
export function SimulationReportView({
  agentId,
  report,
}: {
  agentId: string;
  report: ReturnType<typeof useSimulation>;
}) {
  const { t } = useTranslation();
  const [correcting, setCorrecting] = useState<SimulationCase | null>(null);

  if (report.isLoading) return <LoadingRows rows={6} />;
  if (report.error) {
    return (
      <ErrorState error={report.error} onRetry={() => void report.refetch()} />
    );
  }
  if (!report.data) return null;

  if (report.data.cases.length === 0) {
    return (
      <EmptyState
        title={t("simulation.emptyTitle")}
        hint={t("simulation.emptyBody", { agent: agentId })}
      />
    );
  }

  return (
    <>
      <SimulationSummary report={report.data} />
      {/* Under the tally, above the rows: "did it pass" is answered first,
          and "is it better than the last one" is the question that follows
          it — not one somebody should have to open another screen for. */}
      <VersionComparison agentId={agentId} />
      <ul className="flex flex-col gap-2">
        {report.data.cases.map((entry, i) => (
          <li key={entry.runId ?? entry.id ?? i}>
            <CaseRow
              index={i + 1}
              entry={entry}
              // A case with no run behind it never happened, and there is
              // nothing to say should have been true of it.
              onCorrect={entry.runId ? () => setCorrecting(entry) : undefined}
            />
          </li>
        ))}
      </ul>

      {correcting && (
        <CorrectionDialog
          agentId={agentId}
          entry={correcting}
          onClose={() => setCorrecting(null)}
        />
      )}
    </>
  );
}
