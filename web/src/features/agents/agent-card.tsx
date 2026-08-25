import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
  Activity,
  Bot,
  Clock3,
  CreditCard,
  Database,
  FileText,
  GitBranch,
  Inbox,
  KeyRound,
  LifeBuoy,
  MessageSquare,
  Plug,
  ShieldCheck,
  UserRoundCheck,
  Wrench,
  Zap,
} from "lucide-react";
import { Link } from "react-router-dom";
import { Badge } from "@/components/ui/badge";
import { StateDot } from "@/components/shared/state-dot";
import { stateOfAgent } from "@/lib/agent-state";
import { formatMicros, formatRelative } from "@/lib/format";
import { successRate } from "@/features/agents/activity";
import type { Agent } from "@/features/agents/api";
import type { Tool, ToolEffect } from "@/features/admin/api";
import type { Stage } from "@/features/agents/stage-api";

/**
 * What an agent is allowed to do, at a glance.
 *
 * The capability pack is the whole security story — what is not listed cannot
 * be invoked, whatever the specification asks for — so it is on the card
 * rather than a click away.
 *
 * The card is the link, not the title: it carries everything about an agent
 * except its definition, which is exactly what somebody reading it wants next,
 * and a whole card is a far easier target than a line of text.
 */
export function AgentCard({
  agent,
  catalogue = [],
}: {
  agent: Agent;
  catalogue?: Tool[];
}) {
  const { t } = useTranslation();
  const state = stateOfAgent(agent.activity?.lastPhase);
  const effects = toolEffects(catalogue);
  const packs = capabilityPacks(agent.tools, effects);
  const visiblePacks = packs.slice(0, PACK_CAP);
  const hiddenPacks = Math.max(packs.length - visiblePacks.length, 0);
  const writeCount = agent.tools.filter((tool) =>
    writes(effects.get(tool)),
  ).length;
  const allClassified =
    agent.tools.length > 0 &&
    agent.tools.every((tool) => classified(effects.get(tool)));
  return (
    <Link
      to={hrefFor(agent)}
      className="grid h-[272px] min-w-0 grid-cols-[minmax(0,1fr)] grid-rows-[auto_auto_1fr_auto] overflow-hidden rounded-xl border bg-card shadow-sm transition-colors hover:border-border-strong focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/26"
    >
      <header className="flex min-w-0 items-start gap-2.5 px-4 pt-3 pb-2.5">
        <span className="relative shrink-0">
          <span className="grid size-8 place-items-center rounded-md border bg-muted text-muted-foreground">
            {renderDomainIcon(agent)}
          </span>
          {/* The dot repeats what the footer says in words: an agent's state is
              a fact about its runs, never colour on its own. */}
          <StateDot
            state={state}
            className="absolute right-[-3px] bottom-[-3px] size-[9px] border-2 border-card"
          />
        </span>
        <div className="min-w-0 flex-1 space-y-1">
          <h3 className="truncate text-sm font-medium" title={agent.name}>
            {agent.name}
          </h3>
          <div className="flex min-w-0 items-center gap-2">
            <span
              className="min-w-0 flex-1 truncate font-mono text-[11px] text-muted-foreground"
              title={`${agent.agentId} · ${agent.scope.area}`}
            >
              {agent.agentId} · {agent.scope.area}
            </span>
            <StageChip stage={agent.stage} />
          </div>
        </div>
        {!agent.latest && (
          <Badge
            variant="outline"
            className="h-5 shrink-0 px-1.5 text-[10px] text-muted-foreground"
          >
            {t("agents.oldVersion")}
          </Badge>
        )}
      </header>

      <section className="min-w-0 border-b border-border-subtle px-4 pb-3">
        <p className="mb-1.5 text-[10px] font-semibold tracking-label text-muted-foreground uppercase">
          {t("agents.canUse")}
        </p>
        {agent.tools.length === 0 ? (
          <p className="h-12 text-xs text-muted-foreground">
            {t("agents.withoutTools")}
          </p>
        ) : (
          <div className="h-12 overflow-hidden">
            <div className="flex flex-wrap gap-1.5">
              {visiblePacks.map((pack) => (
                <CapabilityChip key={pack.name} pack={pack} t={t} />
              ))}
              {hiddenPacks > 0 && (
                <span className="inline-flex h-[21px] items-center rounded-sm border border-dashed border-border-strong px-2 font-mono text-[11px] text-muted-foreground">
                  +{hiddenPacks}
                </span>
              )}
            </div>
          </div>
        )}
        <div className="mt-1.5 flex min-w-0 items-center gap-1.5 text-2xs text-muted-foreground">
          <Wrench className="size-3 shrink-0" aria-hidden />
          <span className="truncate">
            {toolSummary(agent.tools.length, writeCount, allClassified, t)}
          </span>
        </div>
      </section>

      <dl className="grid min-w-0 grid-cols-3 gap-2 px-4 py-3 text-xs">
        <Figure
          label={t("runs.runs")}
          value={agent.activity ? String(agent.activity.runs) : "—"}
          primary
        />
        <Figure label={t("runs.finishedPlural")} value={successRate(agent)} />
        <Figure
          label={t("runs.kpiCost")}
          value={
            agent.activity?.costMicros
              ? formatMicros(agent.activity.costMicros)
              : "—"
          }
        />
      </dl>

      <footer className="flex min-w-0 items-center gap-2 border-t border-border-subtle bg-muted/40 px-4 py-2.5 text-2xs text-muted-foreground">
        <span className="inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap">
          <Clock3 className="size-3" aria-hidden />
          {activitySummary(agent, t)}
        </span>
        <span
          className="min-w-0 flex-1 truncate text-right font-mono text-[11px] text-muted-foreground/80 [direction:rtl]"
          title={`${agent.provider}/${agent.model} · ${agent.versionId}`}
        >
          {agent.provider}/{agent.model} · {agent.versionId.slice(0, 9)}
        </span>
      </footer>
    </Link>
  );
}

/**
 * An old version's card describes that version, so it opens that version.
 * Following it to whatever is newest would show a reader text the card they
 * clicked was not about.
 */
function hrefFor(agent: Agent): string {
  const base = `/agents/${agent.agentId}`;
  return agent.latest ? base : `${base}?version=${agent.versionId}`;
}

function Figure({
  label,
  value,
  primary = false,
}: {
  label: string;
  value: string;
  primary?: boolean;
}) {
  return (
    <div className="min-w-0">
      <dt className="truncate text-2xs tracking-label text-muted-foreground uppercase">
        {label}
      </dt>
      <dd
        className={
          primary
            ? "truncate font-mono font-medium tabular-nums"
            : "truncate font-mono text-muted-foreground tabular-nums"
        }
      >
        {value}
      </dd>
    </div>
  );
}

/** What the state dot means, in words.
 *
 *  Takes `t` rather than returning a key: the sentence changes shape with the
 *  numbers in it, so which key applies is a decision this function makes. */
function activitySummary(agent: Agent, t: TFunction): string {
  const activity = agent.activity;
  if (!activity || !activity.lastRunAt) return t("agents.neverRanShort");
  const when = formatRelative(activity.lastRunAt);
  if (activity.waiting > 0) {
    return t("agents.waitingAndLastRun", { count: activity.waiting, when });
  }
  return t("agents.lastRun", { when });
}

const PACK_CAP = 2;

type CapabilityPack = {
  name: string;
  count: number;
  tools: string[];
  canWrite: boolean;
  allRead: boolean;
};

function toolEffects(catalogue: Tool[]): Map<string, ToolEffect> {
  return new Map(catalogue.map((tool) => [tool.toolId, tool.effect]));
}

function capabilityPacks(
  tools: string[],
  effects: Map<string, ToolEffect>,
): CapabilityPack[] {
  const grouped = new Map<string, string[]>();
  for (const tool of tools) {
    const name = packName(tool);
    grouped.set(name, [...(grouped.get(name) ?? []), tool]);
  }
  return [...grouped.entries()]
    .map(([name, list]) => ({
      name,
      count: list.length,
      tools: list,
      canWrite: list.some((tool) => writes(effects.get(tool))),
      allRead:
        list.length > 0 &&
        list.every((tool) => effects.get(tool) === "read"),
    }))
    .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name));
}

function packName(tool: string): string {
  const dot = tool.indexOf(".");
  if (dot > 0) return tool.slice(0, dot);
  return "tools";
}

function CapabilityChip({
  pack,
  t,
}: {
  pack: CapabilityPack;
  t: TFunction;
}) {
  return (
    <span
      title={packTitle(pack, t)}
      className={`inline-flex h-[21px] max-w-full items-center gap-1.5 rounded-sm px-2 pr-1 font-mono text-[11px] leading-none ${packClass(pack)}`}
    >
      {renderPackIcon(pack.name)}
      <span className="min-w-0 truncate leading-none">{pack.name}</span>
      <span className="grid h-[15px] min-w-[17px] place-items-center rounded-[3px] bg-card px-1 text-[10px] font-semibold leading-none text-current tabular-nums">
        {pack.count}
      </span>
    </span>
  );
}

function packClass(pack: CapabilityPack): string {
  if (pack.canWrite) return "border border-primary bg-surface-accent text-text-accent";
  if (pack.allRead) return "border bg-muted text-muted-foreground";
  return "border border-dashed border-border-strong bg-card text-muted-foreground";
}

function toolSummary(
  count: number,
  writeCount: number,
  allClassified: boolean,
  t: TFunction,
): string {
  if (count === 0) return t("agents.reachesNothing");
  const total = t("agents.toolCount", { count });
  if (writeCount > 0) {
    return `${total} · ${t("agents.canWrite", { count: writeCount })}`;
  }
  if (allClassified) return `${total} · ${t("agents.readOnly")}`;
  return total;
}

function writes(effect: ToolEffect | undefined): boolean {
  return (
    effect === "write" || effect === "destructive" || effect === "financial"
  );
}

function classified(effect: ToolEffect | undefined): boolean {
  return effect === "read" || writes(effect);
}

function packTitle(pack: CapabilityPack, t: TFunction): string {
  if (pack.canWrite)
    return t("agents.packTitleWrite", {
      name: pack.name,
      count: pack.count,
    });
  if (pack.allRead)
    return t("agents.packTitleRead", {
      name: pack.name,
      count: pack.count,
    });
  return pack.tools.join(", ");
}

function StageChip({ stage }: { stage: Stage | undefined }) {
  const { t } = useTranslation();
  const shown = stage ?? "draft";
  return (
    <span className="inline-flex h-[19px] shrink-0 items-center gap-1 rounded-sm border bg-muted px-1.5 text-[11px] text-muted-foreground">
      {renderStageIcon(shown)}
      {t(`stage.${shown}`)}
    </span>
  );
}

function renderStageIcon(stage: Stage) {
  switch (stage) {
    case "autonomous":
      return <Zap className="size-3" aria-hidden />;
    case "copilot":
      return <UserRoundCheck className="size-3" aria-hidden />;
    case "draft":
      return <FileText className="size-3" aria-hidden />;
  }
}

function renderPackIcon(name: string) {
  switch (name) {
    case "channel":
      return <MessageSquare className="size-3 shrink-0" aria-hidden />;
    case "github":
      return <GitBranch className="size-3 shrink-0" aria-hidden />;
    case "grafana":
      return <Activity className="size-3 shrink-0" aria-hidden />;
    case "notion":
      return <FileText className="size-3 shrink-0" aria-hidden />;
    case "okta":
      return <KeyRound className="size-3 shrink-0" aria-hidden />;
    case "postgres":
      return <Database className="size-3 shrink-0" aria-hidden />;
    case "stripe":
      return <CreditCard className="size-3 shrink-0" aria-hidden />;
    case "zendesk":
      return <LifeBuoy className="size-3 shrink-0" aria-hidden />;
    default:
      return <Plug className="size-3 shrink-0" aria-hidden />;
  }
}

function renderDomainIcon(agent: Agent) {
  const haystack = `${agent.agentId} ${agent.name} ${agent.scope.area}`.toLowerCase();
  if (haystack.includes("security") || haystack.includes("access"))
    return <ShieldCheck className="size-4" aria-hidden />;
  if (haystack.includes("support") || haystack.includes("triage"))
    return <Inbox className="size-4" aria-hidden />;
  return <Bot className="size-4" aria-hidden />;
}
