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
  const tools = useTools();
  const effects = new Map((tools.data?.items ?? []).map((t) => [t.toolId, t.effect] as const));

  return (
    <section className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 shadow-sm">
      <h2 className="text-2xs uppercase tracking-label text-muted-foreground">
        Pacote de capacidades
      </h2>

      {agent.tools.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          Nenhuma ferramenta. Este agente não consegue tocar em nada fora do modelo.
        </p>
      ) : (
        <ul className="flex flex-col gap-1.5">
          {agent.tools.map((tool) => {
            const effect = effects.get(tool) ?? "read";
            return (
              <li key={tool} className="flex items-center justify-between gap-2">
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
        Teto por execução
      </h2>
      <dl className="flex flex-col gap-1.5">
        <Limit label="Custo" value={budgetOf(agent.budget.micros, formatMicros)} />
        <Limit label="Tokens" value={budgetOf(agent.budget.tokens, formatTokens)} />
        <Limit label="Chamadas" value={budgetOf(agent.budget.toolCalls, String)} />
        <Limit label="Passos" value={budgetOf(agent.budget.steps, String)} />
      </dl>
    </section>
  );
}

/** Zero means no ceiling, which is a different thing from a ceiling of zero. */
function budgetOf(value: number | undefined, format: (n: number) => string): string {
  return value && value > 0 ? format(value) : "sem teto";
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
