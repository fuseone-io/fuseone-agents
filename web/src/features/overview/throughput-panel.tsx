import { useTranslation } from "react-i18next";
import { useMemo } from "react";
import { Skeleton } from "@/components/ui/skeleton";
import { STATE_DOT, type AgentState } from "@/lib/agent-state";
import { cn } from "@/lib/utils";
import {
  ceilingOf,
  columnsFor,
  SEGMENTS,
} from "@/features/overview/throughput-model";
import { ThroughputChart } from "@/features/overview/throughput-chart";
import { useThroughput } from "@/features/overview/api";

const LEGEND: Partial<Record<AgentState, string>> = {
  done: "concluídas",
  waiting: "em curso",
  blocked: "paradas",
};

/**
 * How today went, hour by hour.
 *
 * Stacked rather than three lines: the question is how much ran and what
 * became of it, and a reader comparing three separate curves has to do the
 * addition themselves.
 */
export function ThroughputPanel({ since }: { since: string }) {
  const { t } = useTranslation();
  const { data, isLoading, error } = useThroughput(since);

  const columns = useMemo(
    () => columnsFor(data?.buckets ?? [], since),
    [data?.buckets, since],
  );

  return (
    <section className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 shadow-sm">
      <div className="flex flex-wrap items-center gap-3">
        <h2 className="text-sm font-medium">{t("overview.throughput")}</h2>
        <div className="ml-auto flex items-center gap-3">
          {SEGMENTS.map((state) => (
            <span
              key={state}
              className="flex items-center gap-1.5 text-xs text-muted-foreground"
            >
              <span
                aria-hidden
                className={cn("size-[7px] rounded-[2px]", STATE_DOT[state])}
              />
              {LEGEND[state]}
            </span>
          ))}
        </div>
      </div>

      {isLoading ? (
        <Skeleton className="h-[152px] w-full rounded-lg" />
      ) : error ? (
        <p className="py-16 text-center text-sm text-muted-foreground">
          {t("overview.throughputUnreadable")}
        </p>
      ) : (
        <ThroughputChart columns={columns} ceiling={ceilingOf(columns)} />
      )}
    </section>
  );
}
