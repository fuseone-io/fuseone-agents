import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { SlidersHorizontal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useAgents } from "@/features/agents/api";
import { useThroughput } from "@/features/overview/api";
import { FleetCard } from "@/features/overview/fleet-card";
import { trendsByAgent } from "@/features/overview/fleet-model";

/**
 * Every agent that exists, and how its day went.
 *
 * The fleet rather than the busiest few: an agent that ran nothing today is
 * exactly what somebody scanning this would want to notice, and a list sorted
 * by volume hides it at the bottom.
 */
export function AgentFleet({ since }: { since: string }) {
  const { t } = useTranslation();
  const agents = useAgents();
  const hours = useThroughput(since);

  const items = agents.data?.items ?? [];
  const trends = trendsByAgent(hours.data?.buckets ?? [], since);

  return (
    <section className="flex flex-col gap-2.5">
      <div className="flex items-center gap-2">
        <h2 className="text-sm font-medium">{t("overview.fleet")}</h2>
        <Button
          variant="ghost"
          size="sm"
          asChild
          className="ml-auto h-7 text-muted-foreground"
        >
          <Link to="/agents">
            <SlidersHorizontal className="size-4" aria-hidden />
            {t("overview.manage")}
          </Link>
        </Button>
      </div>

      {agents.isLoading ? (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 3 }, (_, i) => (
            <Skeleton key={i} className="h-[124px] rounded-xl" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <p className="rounded-xl border border-border bg-card p-6 text-center text-sm text-muted-foreground">
          {t("overview.noAgents")}
        </p>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {items.map((agent) => (
            <FleetCard
              key={agent.agentId}
              agent={agent}
              trend={trends.get(agent.agentId) ?? []}
            />
          ))}
        </div>
      )}
    </section>
  );
}
