import { useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { ClipboardCheck, Pencil } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { Mono } from "@/components/shared/mono";
import { CaseRow } from "@/features/agents/simulation-case";
import { CorrectionDialog } from "@/features/agents/correction-dialog";
import { VersionComparison } from "@/features/agents/version-comparison";
import { caseNeedsLook, tally } from "@/features/agents/simulation-tally";
import { formatMicros } from "@/lib/format";
import type { SimulationCase } from "@/features/agents/simulation-api";
import type { useSimulation } from "@/features/agents/simulation-api";

/**
 * The four states a report can be in, and the rows once it has them.
 *
 * The report reads like a rehearsal: what was tried, whether it went as
 * expected, and what answer would have been written. The full trail remains
 * linked from each run for people who need every sealed step.
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

  const counted = tally(report.data);
  const needsLook = report.data.cases.filter(caseNeedsLook).length;
  const expected = Math.max(counted.cases - needsLook, 0);
  const reachedTools = report.data.cases.filter((entry) =>
    (entry.acted ?? []).some((act) => act.reached),
  ).length;

  return (
    <div className="grid min-w-0 gap-5 lg:grid-cols-[minmax(0,1fr)_316px] lg:items-start">
      <main className="flex min-w-0 flex-col gap-4">
        <section className="overflow-hidden rounded-xl border bg-card shadow-sm">
          <header className="flex min-w-0 flex-wrap items-center gap-3 border-b px-4 py-3">
            <div className="min-w-0 flex-1">
              <h2 className="text-sm font-medium">
                {t("simulation.situationsTried")}
              </h2>
              <p className="text-xs text-muted-foreground">
                {report.data.running
                  ? t("simulation.partialResults")
                  : t("simulation.doneResults")}
              </p>
            </div>
            {report.data.running && (
              <span className="rounded-pill border px-2.5 py-1 text-xs text-muted-foreground">
                {t("simulation.stillRunning")}
              </span>
            )}
          </header>
          <ul className="divide-y">
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
        </section>

        <VersionComparison agentId={agentId} />
      </main>

      <aside className="flex min-w-0 flex-col gap-4 lg:sticky lg:top-0">
        <section className="flex min-w-0 flex-col gap-3 rounded-xl border bg-card p-4 shadow-sm">
          <div className="flex items-center gap-2">
            <ClipboardCheck
              className="size-4 text-muted-foreground"
              aria-hidden
            />
            <h2 className="text-2xs uppercase tracking-label text-muted-foreground">
              {t("simulation.howItWent")}
            </h2>
          </div>
          <p className="text-sm">
            {needsLook === 0
              ? t("simulation.allExpected", { count: counted.cases })
              : t("simulation.someNeedLook", {
                  expected,
                  count: counted.cases,
                  needs: needsLook,
                })}
          </p>
          <dl className="flex flex-col gap-2 border-t pt-3">
            <RailFigure label={t("simulation.wentExpected")} value={expected} />
            <RailFigure label={t("simulation.needsLook")} value={needsLook} />
            <RailFigure
              label={t("simulation.wouldHaveActed")}
              value={reachedTools}
            />
            <div className="flex items-center gap-2 pt-1">
              <dt className="flex-1 text-xs text-muted-foreground">
                {t("simulation.tallyCost")}
              </dt>
              <dd>
                <Mono className="text-xs">{formatMicros(counted.micros)}</Mono>
              </dd>
            </div>
          </dl>
          <Button asChild variant="outline" size="sm">
            <Link to={`/agents/${agentId}/edit`}>
              <Pencil className="size-4" aria-hidden />
              {t("simulation.fixInstructions")}
            </Link>
          </Button>
        </section>
      </aside>

      {correcting && (
        <CorrectionDialog
          agentId={agentId}
          entry={correcting}
          onClose={() => setCorrecting(null)}
        />
      )}
    </div>
  );
}

function RailFigure({ label, value }: { label: string; value: number }) {
  return (
    <div className="flex items-center gap-2">
      <dt className="flex-1 text-xs text-muted-foreground">{label}</dt>
      <dd className="font-mono text-sm tabular-nums">{value}</dd>
    </div>
  );
}
