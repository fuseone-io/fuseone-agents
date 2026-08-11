import { useState } from "react";
import { PageHeader } from "@/components/shared/page-header";
import { KpiRow } from "@/features/overview/kpi-row";
import { ThroughputPanel } from "@/features/overview/throughput-panel";
import { CostCeiling } from "@/features/overview/cost-ceiling";
import { DecisionsFeed } from "@/features/overview/decisions-feed";
import { RecentRuns } from "@/features/overview/recent-runs";
import { windowsFor } from "@/features/overview/window";

/**
 * Operational health at a glance.
 *
 * Reading order is the order the questions arrive: how much ran and how it
 * went, when it ran, what it cost, what the rules decided, and then the runs
 * themselves. Everything above the runs table is a comparison — a figure on
 * this screen without something to measure it against is decoration.
 */
export function OverviewPage() {
  // Rounded to the hour and held, so the query keys do not move under the
  // page while somebody is reading it.
  const [windows] = useState(() => windowsFor());

  return (
    <>
      <PageHeader
        title="Visão geral"
        description="Como o dia está indo: quanto rodou, o que o Portão decidiu e quanto custou."
      />

      <KpiRow windows={windows} />

      <div className="grid gap-4 lg:grid-cols-[2fr_1fr] lg:items-start">
        <ThroughputPanel since={windows.current.since} />
        <CostCeiling windows={windows.current} />
      </div>

      <div className="grid gap-4 lg:grid-cols-[1fr_1fr] lg:items-start">
        <DecisionsFeed since={windows.current.since} />
        <RecentRuns since={windows.current.since} />
      </div>
    </>
  );
}
