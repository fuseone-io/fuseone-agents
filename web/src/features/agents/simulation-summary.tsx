import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Panel } from "@/components/shared/panel";
import { Mono } from "@/components/shared/mono";
import { tally } from "@/features/agents/simulation-tally";
import { formatMicros } from "@/lib/format";
import type { SimulationReport } from "@/features/agents/simulation-api";

/**
 * The line an author reads before any of the rows.
 *
 * Cases the Gate refused are counted apart from where each one ended: a run
 * that was refused and carried on is still one somebody has to look at, and
 * folding it into "finished" would hide exactly what the simulation was run
 * to find.
 */
export function SimulationSummary({ report }: { report: SimulationReport }) {
  const { t } = useTranslation();
  const counted = tally(report);

  return (
    <Panel
      title={t("simulation.reportTitle")}
      action={
        report.running ? (
          <Badge variant="outline">{t("simulation.stillRunning")}</Badge>
        ) : undefined
      }
    >
      <dl className="flex flex-wrap gap-x-8 gap-y-3">
        {(report.held ?? 0) + (report.broken ?? 0) > 0 && (
          <>
            <Figure
              label={t("correction.tallyHeld")}
              value={report.held ?? 0}
            />
            <Figure
              label={t("correction.tallyBroken")}
              value={report.broken ?? 0}
            />
          </>
        )}
        <Figure label={t("simulation.tallyCases")} value={counted.cases} />
        <Figure
          label={t("simulation.tallyFinished")}
          value={counted.finished}
        />
        <Figure label={t("simulation.tallyParked")} value={counted.parked} />
        <Figure label={t("simulation.tallyWaiting")} value={counted.waiting} />
        <Figure label={t("simulation.tallyStopped")} value={counted.stopped} />
        {counted.running > 0 && (
          <Figure
            label={t("simulation.tallyRunning")}
            value={counted.running}
          />
        )}

        <div className="flex flex-col gap-0.5">
          <dt className="text-2xs uppercase tracking-label text-muted-foreground">
            {t("simulation.tallyCost")}
          </dt>
          <dd>
            <Mono className="text-lg">{formatMicros(counted.micros)}</Mono>
          </dd>
        </div>
      </dl>
    </Panel>
  );
}

function Figure({ label, value }: { label: string; value: number }) {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="text-2xs uppercase tracking-label text-muted-foreground">
        {label}
      </dt>
      <dd className="text-lg tabular-nums">{value}</dd>
    </div>
  );
}
