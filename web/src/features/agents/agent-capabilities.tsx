import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useTools } from "@/features/admin/api";
import { EffectBadge } from "@/features/agents/effect-badge";
import { Mono } from "@/components/shared/mono";
import { formatMicros, formatTokens } from "@/lib/format";
import type { Agent } from "@/lib/api/client";

export function AgentCapabilities({ agent, compact = false }: {
  agent: Agent;
  compact?: boolean;
}) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const tools = useTools();
  const effects = new Map(
    (tools.data?.items ?? []).map((t) => [t.toolId, t.effect] as const),
  );
  const shownTools = compact && !expanded ? agent.tools.slice(0, 6) : agent.tools;
  const hidden = Math.max(agent.tools.length - shownTools.length, 0);

  return (
    <section className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 shadow-sm">
      <h2 className="text-2xs uppercase tracking-label text-muted-foreground">
        {t("agents.capabilities")}
      </h2>

      {agent.tools.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t("agents.noTools")}</p>
      ) : (
        <ul
          className={
            compact ? "flex flex-wrap gap-1.5" : "flex flex-col gap-1.5"
          }
        >
          {shownTools.map((tool) => {
            const effect = effects.get(tool) ?? "unknown";
            return (
              <li
                key={tool}
                className={
                  compact
                    ? "inline-flex min-w-0 max-w-full items-center gap-1.5 rounded-md bg-muted px-2 py-1"
                    : "flex items-center justify-between gap-2"
                }
              >
                <Mono className="truncate">{tool}</Mono>
                <EffectBadge effect={effect} />
              </li>
            );
          })}
          {hidden > 0 && (
            <li>
              <button
                type="button"
                className="inline-flex items-center rounded-md bg-muted px-2 py-1 text-2xs text-muted-foreground transition hover:bg-muted/80 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                onClick={() => setExpanded(true)}
              >
                {t("agents.moreTools", { count: hidden })}
              </button>
            </li>
          )}
        </ul>
      )}

      <h2 className="mt-1 text-2xs uppercase tracking-label text-muted-foreground">
        {t("agents.whatStarts")}
      </h2>
      <Triggers agent={agent} />

      <h2 className="mt-1 text-2xs uppercase tracking-label text-muted-foreground">
        {t("agents.perRunCeiling")}
      </h2>
      <dl className="flex flex-col gap-1.5">
        <Limit
          label={t("runs.kpiCost")}
          value={budgetOf(agent.budget.micros, formatMicros, t)}
        />
        <Limit
          label={t("runs.kpiTokens")}
          value={budgetOf(agent.budget.tokens, formatTokens, t)}
        />
        <Limit
          label={t("agents.calls")}
          value={budgetOf(agent.budget.toolCalls, String, t)}
        />
        <Limit
          label={t("runs.columnSteps")}
          value={budgetOf(agent.budget.steps, String, t)}
        />
      </dl>
    </section>
  );
}

function Triggers({ agent }: { agent: Agent }) {
  const { t } = useTranslation();
  const triggers = agent.triggers ?? [];
  if (triggers.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">{t("agents.noTriggers")}</p>
    );
  }
  return (
    <ul className="flex flex-col gap-1.5">
      {triggers.map((trigger, i) => (
        <li
          key={`${trigger.type}-${i}`}
          className="flex items-baseline justify-between gap-3"
        >
          <span className="text-xs text-muted-foreground">{trigger.type}</span>
          <Mono>
            {trigger.schedule ?? trigger.path ?? trigger.event ?? "—"}
          </Mono>
        </li>
      ))}
    </ul>
  );
}

function budgetOf(
  value: number | undefined,
  format: (n: number) => string,
  t: (key: string) => string,
): string {
  return value && value > 0 ? format(value) : t("agents.noCeiling");
}

function Limit({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd>
        <Mono>{value}</Mono>
      </dd>
    </div>
  );
}
