import { useTranslation } from "react-i18next";
import { useDeferredValue, useMemo, useState } from "react";
import { Activity } from "lucide-react";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { Panel } from "@/components/shared/panel";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { RunsFilters, sinceFor } from "@/features/runs/runs-filters";
import { RunsKpis } from "@/features/runs/runs-kpis";
import { RunsTable } from "@/features/runs/runs-table";
import { useRunStats, useRuns } from "@/features/runs/api";
import type { Phase } from "@/lib/api/client";

export function RunsPage() {
  const { t } = useTranslation();
  const [search, setSearch] = useState("");
  const [phase, setPhase] = useState<Phase | "all">("all");
  const [period, setPeriod] = useState("7");
  const query = useDeferredValue(search.trim());

  // Recomputed only when the period changes. sinceFor is stable within a
  // minute anyway; this keeps the query key from churning at all.
  const since = useMemo(() => sinceFor(period), [period]);

  const { data, isLoading, error, refetch } = useRuns({
    phase: phase === "all" ? undefined : phase,
    since,
    q: query || undefined,
  });

  // Deliberately not narrowed by phase: the row exists to say how the whole
  // period is going, and a "concluded" share that moved with the phase filter
  // would always read 100%.
  const stats = useRunStats({ since });

  const runs = data?.items ?? [];

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.runs}
        title={t("runs.runs")}
        description={t("runs.subtitle")}
      />

      <RunsKpis stats={stats.data} isLoading={stats.isLoading} />

      <RunsFilters
        search={search}
        phase={phase}
        period={period}
        onSearch={setSearch}
        onPhase={setPhase}
        onPeriod={setPeriod}
      />

      <Panel
        title={t("runs.runs")}
        action={
          <span className="text-xs text-muted-foreground tabular-nums">
            {t("runs.runCount", { count: runs.length })}
          </span>
        }
        flush
      >
        <Body
          isLoading={isLoading}
          error={error}
          onRetry={() => void refetch()}
          empty={runs.length === 0 ? <Nothing query={query} /> : undefined}
        >
          <RunsTable runs={runs} />
        </Body>
      </Panel>
    </>
  );
}

/** The four states every view that loads data owes the reader. */
function Body({
  isLoading,
  error,
  onRetry,
  empty,
  children,
}: {
  isLoading: boolean;
  error: unknown;
  onRetry: () => void;
  empty?: React.ReactNode;
  children: React.ReactNode;
}) {
  if (isLoading)
    return (
      <div className="p-4">
        <LoadingRows />
      </div>
    );
  if (error)
    return (
      <div className="p-4">
        <ErrorState error={error} onRetry={onRetry} />
      </div>
    );
  if (empty) return <div className="p-4">{empty}</div>;
  return <>{children}</>;
}

/** An empty result says which of the two things happened: nothing matched the
 *  search, or nothing ran in the period. They call for different actions. */
function Nothing({ query }: { query: string }) {
  const { t } = useTranslation();
  return (
    <EmptyState
      icon={<Activity className="size-6" />}
      title={query ? "Nada encontrado" : t("runs.noneInPeriod")}
      hint={query ? t("runs.noMatchFor", { query }) : t("runs.emptyHint")}
    />
  );
}
