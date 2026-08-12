import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Sparkline } from "@/components/shared/sparkline";
import { StateDot } from "@/components/shared/state-dot";
import { costPerRun, successRate } from "@/features/agents/activity";
import { stateOfAgent } from "@/lib/agent-state";
import { formatCost } from "@/lib/format";
import type { Agent, Phase } from "@/lib/api/client";

/**
 * One agent's day: what it is doing, how much it ran, how much of that worked
 * and what each run cost.
 *
 * Cost per run rather than total, because the total only says the agent was
 * busy. The per-run figure is the one that says whether it is expensive.
 */
export function FleetCard({ agent, trend }: { agent: Agent; trend: number[] }) {
  const { t } = useTranslation();
  const activity = agent.activity;
  const runs = activity?.runs ?? 0;
  const perRun = costPerRun(agent);

  return (
    <Link
      to={`/agents/${agent.agentId}`}
      className="flex flex-col gap-2.5 rounded-xl border border-border bg-card p-4 shadow-sm transition-colors hover:bg-muted/40"
    >
      <div className="flex items-center gap-2">
        <StateDot
          state={stateOfAgent(activity?.lastPhase as Phase | undefined)}
        />
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium">{agent.name}</div>
          <div className="truncate text-xs text-muted-foreground">
            {agent.scope.area || "—"}
          </div>
        </div>
      </div>

      {/* A flat line for an agent that ran nothing would claim a measurement
          there was nothing to measure. */}
      {trend.some(Boolean) ? (
        <Sparkline
          points={trend}
          width={200}
          height={28}
          className="w-full text-primary"
        />
      ) : (
        <div className="h-7 content-center text-xs text-muted-foreground">
          {t("overview.nothingToday")}
        </div>
      )}

      <div className="flex flex-wrap gap-3.5 font-mono text-2xs tabular-nums text-muted-foreground">
        <span>
          <span className="text-foreground">{runs}</span>
          {t("overview.runsShort")}
        </span>
        <span>
          <span className="text-foreground">{successRate(agent)}</span>
          {t("overview.okShort")}
        </span>
        <span>
          <span className="text-foreground">
            {perRun === undefined ? "—" : formatCost({ micros: perRun })}
          </span>
          {t("overview.perRun")}
        </span>
      </div>
    </Link>
  );
}
