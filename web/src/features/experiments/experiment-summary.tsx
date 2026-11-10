import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { ExperimentConfigText } from "@/features/experiments/experiment-config";
import { formatMicros } from "@/lib/format";
import type { ExperimentSummary } from "@/features/experiments/experiments-api";

/**
 * What an experiment ran, then what it found (NT-015, "Reading an experiment").
 *
 * In that order and never the other way round: a count of broken cases read
 * before the reader knows it was three cases of forty, on another model, is a
 * number nobody can weigh.
 */
export function ExperimentSummaryFacts({ summary }: { summary: ExperimentSummary }) {
  const { t } = useTranslation();

  return (
    <dl className="grid grid-cols-2 gap-x-8 gap-y-3 sm:grid-cols-3 lg:grid-cols-5">
      <Fact label={t("experiments.datasetVersion")}>
        <Mono>{summary.datasetVersion}</Mono>
      </Fact>
      <Fact label={t("experiments.agentVersion")}>
        <Mono>{summary.version.slice(0, 9)}</Mono>
      </Fact>
      <Fact label={t("experiments.configuration")}>
        <ExperimentConfigText config={summary.config} />
      </Fact>
      <Fact label={t("experiments.casesRun")}>
        {t("experiments.casesRunOf", {
          run: summary.casesRun,
          count: summary.casesInVersion,
        })}
      </Fact>
      <Fact label={t("experiments.erased")}>{summary.erased}</Fact>
      <Fact label={t("experiments.repetitions")}>{summary.repetitions}</Fact>
      <Fact label={t("experiments.held")}>{summary.held}</Fact>
      <Fact label={t("experiments.broken")}>{summary.broken}</Fact>
      <Fact label={t("experiments.cost")}>
        <Mono>{formatMicros(summary.cost.micros)}</Mono>
      </Fact>
    </dl>
  );
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5">
      <dt className="text-2xs uppercase tracking-label text-muted-foreground">{label}</dt>
      <dd className="text-sm tabular-nums">{children}</dd>
    </div>
  );
}
