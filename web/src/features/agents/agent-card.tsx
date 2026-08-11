import { Badge } from "@/components/ui/badge";
import { Mono } from "@/components/shared/mono";
import { StateDot } from "@/components/shared/state-dot";
import { stateOfAgent } from "@/lib/agent-state";
import { formatMicros, formatRelative } from "@/lib/format";
import { successRate } from "@/features/agents/activity";
import type { Agent } from "@/features/agents/api";

/**
 * What an agent is allowed to do, at a glance.
 *
 * The capability pack is the whole security story — what is not listed cannot
 * be invoked, whatever the specification asks for — so it is on the card
 * rather than a click away.
 */
export function AgentCard({ agent }: { agent: Agent }) {
  return (
    <article className="flex flex-col gap-3 rounded-xl border bg-card p-4 shadow-sm">
      <header className="flex items-start gap-2">
        {/* The dot repeats what the footer says in words: an agent's state is
            a fact about its runs, never colour on its own. */}
        <StateDot state={stateOfAgent(agent.activity?.lastPhase)} className="mt-[7px]" />
        <div className="min-w-0 flex-1">
          <h3 className="truncate font-medium">{agent.name}</h3>
          <div className="truncate text-xs text-muted-foreground">
            <Mono dim>{agent.agentId}</Mono> · {agent.scope.area}
          </div>
        </div>
        {!agent.latest && (
          <Badge variant="outline" className="text-muted-foreground">
            versão antiga
          </Badge>
        )}
      </header>

      <div className="flex flex-wrap gap-1">
        {agent.tools.length === 0 ? (
          <span className="text-xs text-muted-foreground">Sem ferramentas</span>
        ) : (
          agent.tools.map((tool) => (
            <Badge key={tool} variant="secondary" className="font-mono text-2xs font-normal">
              {tool}
            </Badge>
          ))
        )}
      </div>

      <dl className="grid grid-cols-3 gap-2 border-t border-border-subtle pt-3 text-xs">
        <Figure label="Execuções" value={agent.activity ? String(agent.activity.runs) : "—"} />
        <Figure label="Concluídas" value={successRate(agent)} />
        <Figure label="Custo" value={agent.activity?.costMicros ? formatMicros(agent.activity.costMicros) : "—"} />
      </dl>

      <dl className="grid grid-cols-3 gap-2 text-xs">
        <Figure label="Teto" value={agent.budget.micros ? formatMicros(agent.budget.micros) : "—"} />
        <Figure label="Passos" value={agent.budget.steps ? String(agent.budget.steps) : "—"} />
        <Figure label="Gatilhos" value={triggerSummary(agent)} />
      </dl>

      <footer className="flex flex-col gap-1 border-t border-border-subtle pt-3 text-2xs text-muted-foreground">
        <span>{activitySummary(agent)}</span>
        <span>
          <Mono dim>
            {agent.provider}/{agent.model}
          </Mono>{" "}
          · <Mono dim>{agent.versionId.slice(0, 9)}</Mono>
        </span>
      </footer>
    </article>
  );
}

function Figure({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-2xs uppercase tracking-label text-muted-foreground">{label}</dt>
      <dd className="font-mono tabular-nums">{value}</dd>
    </div>
  );
}

/** What the state dot means, in words. */
function activitySummary(agent: Agent): string {
  const activity = agent.activity;
  if (!activity || !activity.lastRunAt) return "Nunca executou";
  if (activity.waiting > 0) {
    return `${activity.waiting} esperando pessoa · última execução ${formatRelative(activity.lastRunAt)}`;
  }
  return `Última execução ${formatRelative(activity.lastRunAt)}`;
}

/** Says how a run starts, because an agent nothing triggers never runs. */
function triggerSummary(agent: Agent): string {
  const triggers = agent.triggers ?? [];
  if (triggers.length === 0) return "manual";
  return [...new Set(triggers.map((t) => t.type))].join(", ");
}
