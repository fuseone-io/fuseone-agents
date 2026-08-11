import { Gauge } from "lucide-react";
import { Link } from "react-router-dom";
import { Panel } from "@/components/shared/panel";
import { EmptyState } from "@/components/shared/states";
import { formatMicros } from "@/lib/format";
import { scopeLabel, scopePath } from "@/features/admin/budget-scope";
import { useBudgets } from "@/features/admin/api";
import type { components } from "@/lib/api/schema.gen";

type CostRollup = components["schemas"]["CostRollup"];

/**
 * Spend against the ceiling that governs it.
 *
 * The bar turns amber past three quarters, because the number that matters is
 * not what was spent but how close it is to stopping runs — a ceiling reached
 * pauses every run in the scope until somebody raises it.
 */
export function CostCaps({ byArea }: { byArea?: CostRollup }) {
  const { data } = useBudgets();
  const budgets = (data?.items ?? []).filter((b) => b.enabled && b.micros);

  if (budgets.length === 0) {
    return (
      <Panel title="Tetos">
        <EmptyState
          icon={<Gauge className="size-6" />}
          title="Nenhum teto configurado"
          hint="Sem teto por escopo, o único limite é o de cada agente por execução. Configure um em Administração → Tetos."
        />
      </Panel>
    );
  }

  const spentByArea = new Map((byArea?.buckets ?? []).map((b) => [b.key, b.cost.micros]));
  const total = (byArea?.buckets ?? []).reduce((sum, b) => sum + b.cost.micros, 0);

  return (
    <Panel
      title="Tetos"
      action={
        <Link to="/admin" className="text-xs text-text-accent underline-offset-4 hover:underline">
          Configurar
        </Link>
      }
    >
      <ul className="flex flex-col gap-3">
        {budgets.map((budget) => {
          const path = scopePath(budget);
          // An area ceiling is measured against that area; anything wider is
          // measured against everything the window covers.
          const spent = budget.scope?.area ? (spentByArea.get(budget.scope.area) ?? 0) : total;
          const cap = budget.micros ?? 0;
          const share = cap === 0 ? 0 : Math.min(Math.round((spent / cap) * 100), 100);
          const near = share >= 75;

          return (
            <li key={path}>
              <div className="mb-1 flex justify-between gap-2 text-xs">
                <span>{scopeLabel(budget)}</span>
                <span className="whitespace-nowrap font-mono tabular-nums text-muted-foreground">
                  {formatMicros(spent)} de {formatMicros(cap)} · {share}%
                </span>
              </div>
              <div className="h-1.5 overflow-hidden rounded-pill bg-muted">
                <div
                  className={near ? "h-full rounded-pill bg-warning" : "h-full rounded-pill bg-primary"}
                  style={{ width: `${share}%` }}
                />
              </div>
            </li>
          );
        })}
      </ul>
    </Panel>
  );
}
