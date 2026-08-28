import { useTranslation } from "react-i18next";
import { costPerRun } from "@/features/agents/activity";
import { formatCost, formatRelative } from "@/lib/format";
import type { Agent } from "@/lib/api/client";

/**
 * The agent's health line.
 *
 * The old page spent four full cards on a few numbers and pushed the runs
 * below the definition. These are still useful facts, but they are readings
 * beside the state, not the main content.
 */
export function AgentOverviewSummary({ agent }: { agent: Agent }) {
  const { t } = useTranslation();
  const activity = agent.activity;
  const averageCost = costPerRun(agent);

  const stats = [
    {
      label: t("runs.runs"),
      value: String(activity?.runs ?? 0),
      note: t("agents.sinceFirstVersion"),
    },
    {
      label: t("runs.finishedPlural"),
      value: String(activity?.finished ?? 0),
      note: t("common.ofTotal", {
        count: activity?.finished ?? 0,
        total: activity?.runs ?? 0,
      }),
    },
    {
      label: t("agents.waitingPeople"),
      value: String(activity?.waiting ?? 0),
      note: activity?.lastRunAt
        ? t("agents.lastRun", { when: formatRelative(activity.lastRunAt) })
        : t("agents.neverRanLower"),
    },
    {
      label: t("agents.averageCostPerRun"),
      value: averageCost === undefined ? "—" : formatCost({ micros: averageCost }),
      note: t("agents.totalCostAcrossRuns", {
        total: formatCost({ micros: activity?.costMicros ?? 0 }),
        count: activity?.runs ?? 0,
      }),
    },
  ];

  return (
    <section
      aria-label={t("agents.healthSummary")}
      className="rounded-xl border border-border bg-card px-4 py-3 shadow-sm"
    >
      <dl className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {stats.map((stat) => (
          <div key={stat.label} className="min-w-0">
            <dt className="text-2xs uppercase tracking-label text-muted-foreground">
              {stat.label}
            </dt>
            <dd className="mt-1 font-mono text-base font-medium tabular-nums">
              {stat.value}
            </dd>
            <p className="truncate text-2xs text-muted-foreground">
              {stat.note}
            </p>
          </div>
        ))}
      </dl>
    </section>
  );
}
