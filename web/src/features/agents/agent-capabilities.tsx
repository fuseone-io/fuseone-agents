import { useTranslation } from "react-i18next";
import { useTools } from "@/features/admin/api";
import { Mono } from "@/components/shared/mono";
import { formatMicros, formatTokens } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Agent } from "@/lib/api/client";

const EFFECT_TONE: Record<string, string> = {
  read: "border-border",
  write: "border-warning bg-warning-surface text-warning",
  destructive: "border-danger bg-danger-surface text-danger",
  financial: "border-danger bg-danger-surface text-danger",
};

const EFFECT_LABEL: Record<string, string> = {
  read: "leitura",
  write: "escrita",
  destructive: "destrutivo",
  financial: "financeiro",
};

/**
 * What this version may call, and what it may spend doing it.
 *
 * The effect beside each tool, because the pack is only half the answer: what
 * is not listed here cannot be invoked, and what is listed still has to say
 * what it does to the world before a reader can judge the risk.
 */
export function AgentCapabilities({ agent }: { agent: Agent }) {
  const { t } = useTranslation();
  const tools = useTools();
  const effects = new Map(
    (tools.data?.items ?? []).map((t) => [t.toolId, t.effect] as const),
  );

  return (
    <section className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 shadow-sm">
      <h2 className="text-2xs uppercase tracking-label text-muted-foreground">
        {t("agents.capabilities")}
      </h2>

      {agent.tools.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t("agents.noTools")}</p>
      ) : (
        <ul className="flex flex-col gap-1.5">
          {agent.tools.map((tool) => {
            const effect = effects.get(tool) ?? "read";
            return (
              <li
                key={tool}
                className="flex items-center justify-between gap-2"
              >
                <Mono className="truncate">{tool}</Mono>
                <span
                  className={cn(
                    "shrink-0 rounded-md border px-1.5 text-2xs",
                    EFFECT_TONE[effect] ?? EFFECT_TONE.read,
                  )}
                >
                  {EFFECT_LABEL[effect] ?? effect}
                </span>
              </li>
            );
          })}
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
          value={budgetOf(agent.budget.micros, formatMicros)}
        />
        <Limit
          label={t("runs.kpiTokens")}
          value={budgetOf(agent.budget.tokens, formatTokens)}
        />
        <Limit
          label="Chamadas"
          value={budgetOf(agent.budget.toolCalls, String)}
        />
        <Limit
          label={t("runs.columnSteps")}
          value={budgetOf(agent.budget.steps, String)}
        />
      </dl>
    </section>
  );
}

/**
 * What starts a run of this agent.
 *
 * An agent nothing triggers only ever runs when somebody presses the button,
 * which is worth saying plainly rather than leaving as an empty row.
 */
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

/** Zero means no ceiling, which is a different thing from a ceiling of zero. */
function budgetOf(
  value: number | undefined,
  format: (n: number) => string,
): string {
  return value && value > 0 ? format(value) : "agents.noCeiling";
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
