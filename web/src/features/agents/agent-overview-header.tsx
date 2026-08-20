import { type ReactNode } from "react";
import { Trans, useTranslation } from "react-i18next";
import { Eye } from "lucide-react";
import { Separator } from "@/components/ui/separator";
import { Mono } from "@/components/shared/mono";
import { StateDot } from "@/components/shared/state-dot";
import { HeaderActions } from "@/features/agents/agent-overview-header-actions";
import { StageControl } from "@/features/agents/stage-control";
import { formatCost, formatInstant, formatRelative } from "@/lib/format";
import { stateOfAgent } from "@/lib/agent-state";
import type { Agent, AgentVersion, Phase } from "@/lib/api/client";
import type { Stage } from "@/features/agents/stage-api";

const MEANS: Record<Stage, string> = {
  draft: "stage.means.draft",
  copilot: "stage.means.copilot",
  autonomous: "stage.means.autonomous",
};

export function AgentOverviewHeader({
  agent,
  versions,
  tabs,
}: {
  agent: Agent;
  versions: AgentVersion[];
  tabs: ReactNode;
}) {
  const { t } = useTranslation();
  const superseded = !agent.latest && versions.length > 1;

  return (
    <header className="flex min-w-0 flex-col gap-3 border-b border-border pb-0">
      <div className="flex min-w-0 flex-wrap items-start gap-3">
        <div className="min-w-0 flex-1 space-y-1.5">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <h1 className="min-w-0 truncate text-2xl font-medium tracking-display">
              {agent.name}
            </h1>
            <span className="inline-flex h-6 items-center gap-1.5 rounded-pill bg-muted px-2.5 text-xs font-medium">
              <StateDot
                state={stateOfAgent(
                  agent.activity?.lastPhase as Phase | undefined,
                )}
              />
              {agent.scope.area || t("agents.noArea")}
            </span>
            {superseded && (
              <span className="inline-flex h-6 items-center rounded-pill bg-warning-surface px-2.5 text-xs font-medium text-warning">
                {t("agents.oldVersion")}
              </span>
            )}
          </div>
          <div className="flex min-w-0 flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <Mono dim>{agent.agentId}</Mono>
            <Separator orientation="vertical" className="!h-3" />
            <span className="min-w-0">
              <Trans
                i18nKey="agents.publishedLine"
                values={{
                  version: agent.versionId.slice(0, 9),
                  model: `${agent.provider}/${agent.model}`,
                  by: agent.publishedBy ?? "",
                  at: formatInstant(agent.publishedAt),
                }}
                components={{ v: <Mono dim /> }}
              />
            </span>
          </div>
        </div>

        <HeaderActions agent={agent} />
      </div>

      <div className="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-3">
        <AgentStatus agent={agent} />
        <Separator orientation="vertical" className="hidden !h-7 sm:block" />
        <StageControl agentId={agent.agentId} stage={agent.stage} />
        <span className="flex min-w-0 items-center gap-1.5 text-2xs text-muted-foreground">
          <Eye className="size-3.5 shrink-0" aria-hidden />
          <span className="truncate">
            {t(MEANS[(agent.stage ?? "draft") as Stage])}
          </span>
        </span>
        <HeaderStats agent={agent} />
      </div>

      {tabs}
    </header>
  );
}

function AgentStatus({ agent }: { agent: Agent }) {
  const { t } = useTranslation();
  const retired = agent.retired ?? false;
  const running = !retired && agent.paused === false;
  return (
    <div className="flex min-w-0 items-center gap-2.5">
      <StateDot state={retired ? "draft" : running ? "running" : "blocked"} />
      <div className="min-w-0">
        <p className="text-sm font-medium whitespace-nowrap">
          {retired
            ? t("agents.retiredState")
            : running
              ? t("agents.running")
              : t("agents.stopped")}
        </p>
        <p className="text-2xs whitespace-nowrap text-muted-foreground">
          {agent.activity?.lastRunAt
            ? t("agents.lastRunAt", {
                when: formatRelative(agent.activity.lastRunAt),
              })
            : t("agents.neverRan")}
        </p>
      </div>
    </div>
  );
}

function HeaderStats({ agent }: { agent: Agent }) {
  const { t } = useTranslation();
  const activity = agent.activity;
  const stats = [
    [
      t("runs.runs"),
      String(activity?.runs ?? 0),
      t("agents.sinceFirstVersion"),
    ],
    [
      t("runs.finishedPlural"),
      String(activity?.finished ?? 0),
      t("common.ofTotal", {
        count: activity?.finished ?? 0,
        total: activity?.runs ?? 0,
      }),
    ],
    [
      t("agents.waitingPeople"),
      String(activity?.waiting ?? 0),
      activity?.lastRunAt
        ? t("agents.lastRun", { when: formatRelative(activity.lastRunAt) })
        : t("agents.neverRanLower"),
    ],
    [
      t("agents.costPerRun"),
      formatCost({ micros: activity?.costMicros ?? 0 }),
      t("agents.totalRecorded"),
    ],
  ];

  return (
    <dl className="grid min-w-0 flex-1 grid-cols-2 gap-x-5 gap-y-2 lg:grid-cols-4">
      {stats.map(([label, value, note]) => (
        <div key={label} className="min-w-0">
          <dt className="text-2xs uppercase tracking-label text-muted-foreground">
            {label}
          </dt>
          <dd className="font-mono text-sm font-medium tabular-nums">{value}</dd>
          <p className="truncate text-2xs text-muted-foreground">{note}</p>
        </div>
      ))}
    </dl>
  );
}
