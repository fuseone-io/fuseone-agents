import { useState } from "react";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { KpiRow } from "@/features/overview/kpi-row";
import { ThroughputPanel } from "@/features/overview/throughput-panel";
import { BudgetDonut } from "@/features/overview/budget-donut";
import { AgentFleet } from "@/features/overview/agent-fleet";
import { DecisionsFeed } from "@/features/overview/decisions-feed";
import { RecentRuns } from "@/features/overview/recent-runs";
import { TracePanel } from "@/features/overview/trace-panel";
import { useSuggestedRun } from "@/features/overview/suggested-run";
import { windowsFor } from "@/features/overview/window";

/**
 * Operational health at a glance.
 *
 * Reading order is the order the questions arrive: how the day is going, when
 * it happened and what it cost, who is doing it, what the rules decided, and
 * then the runs themselves. Everything above the table is a comparison — a
 * figure on this screen with nothing to measure it against is decoration.
 */
export function OverviewPage() {
  // Rounded to the hour and held, so the query keys do not move under the
  // page while somebody is reading it.
  const [windows] = useState(() => windowsFor());
  const [chosen, setChosen] = useState<string>();
  const [dismissed, setDismissed] = useState(false);
  const { since } = windows.current;

  // Open on its own, on the run most worth looking at. The panel exists so
  // somebody can act, so it opens on one waiting for a person before one that
  // merely finished — and stays closed once somebody closes it, because a
  // panel that reopens itself is a panel that cannot be closed.
  const suggested = useSuggestedRun(since);
  const selected = chosen ?? (dismissed ? undefined : suggested);

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.overview}
        title="Visão geral"
        description="Como o dia está indo: quanto rodou, o que o Portão decidiu e quanto custou."
      />

      <KpiRow windows={windows} />

      <div className="grid gap-4 lg:grid-cols-[2fr_1fr] lg:items-start">
        <ThroughputPanel since={since} />
        <BudgetDonut windows={windows.current} />
      </div>

      <AgentFleet since={since} />

      {/* The trace docks beside the feed and the table rather than replacing
          them: somebody opening a run should not lose the page they were
          reading it from. */}
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start">
        <div className="flex min-w-0 flex-1 flex-col gap-4">
          <DecisionsFeed since={since} />
          <RecentRuns since={since} selected={selected} onSelect={setChosen} />
        </div>

        {selected && (
          <TracePanel
            runId={selected}
            onClose={() => {
              setChosen(undefined);
              setDismissed(true);
            }}
          />
        )}
      </div>
    </>
  );
}
