import { useTranslation } from "react-i18next";
import type { ReactNode } from "react";
import { formatCost, formatDuration, formatTokens } from "@/lib/format";
import type { Run } from "@/lib/api/client";

/**
 * The four numbers that say how the run is going.
 *
 * Never a figure without its denominator: a cost is only meaningful against
 * the cap, a token count against its split, a step count against how far the
 * run can go. A bare number invites the reader to invent the comparison.
 */
export function RunKpis({ run, steps }: { run: Run; steps: number }) {
  const { t } = useTranslation();
  const input = run.cost.inputTokens ?? 0;
  const output = run.cost.outputTokens ?? 0;
  const reserved = run.reservedMicros ?? 0;

  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <Kpi label={t("runs.kpiCost")} value={formatCost(run.cost)}>
        {reserved > 0
          ? t("runs.stillReserved", { amount: formatCost({ micros: reserved }) })
          : t("runs.nothingReserved")}
      </Kpi>

      <Kpi label={t("runs.kpiTokens")} value={formatTokens(input + output)}>
        {t("runs.tokensSplit", {
          input: formatTokens(input),
          output: formatTokens(output),
        })}
      </Kpi>

      <Kpi label={t("runs.columnSteps")} value={String(steps)}>
        <span className="text-muted-foreground">
          {t("runs.lastSealed", { seq: run.seq })}
        </span>
      </Kpi>

      <Kpi
        label={t("runs.kpiDuration")}
        value={formatDuration(run.startedAt, run.endedAt)}
      >
        {run.endedAt ? t("runs.finished") : t("runs.inProgress")}
      </Kpi>
    </div>
  );
}

function Kpi({
  label,
  value,
  children,
}: {
  label: string;
  value: string;
  children: ReactNode;
}) {
  return (
    <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
      <div className="text-2xs uppercase tracking-label text-muted-foreground">
        {label}
      </div>
      <div className="mt-1.5 font-mono text-[22px]/7 font-medium tabular-nums">
        {value}
      </div>
      <div className="mt-0.5 text-xs text-muted-foreground">{children}</div>
    </div>
  );
}
