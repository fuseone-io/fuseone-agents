import { Link } from "react-router-dom";
import { Skeleton } from "@/components/ui/skeleton";
import { Mono } from "@/components/shared/mono";
import { useBudgets } from "@/features/admin/api";
import { useCostRollup } from "@/features/cost/api";
import { scopeLabel } from "@/features/admin/budget-scope";
import { formatCost } from "@/lib/format";
import { cn } from "@/lib/utils";

/**
 * Today's spend against the ceilings that govern it.
 *
 * The share is of configured ceilings, not of a number invented for the
 * screen: a run may be governed by a company limit, an area limit or neither,
 * so with nothing configured this panel says so rather than printing a
 * percentage with no denominator.
 *
 * The bars turn amber past three quarters, because what matters is not what
 * was spent but how close it is to stopping runs — a ceiling reached pauses
 * every run in its scope until somebody raises it.
 */
export function BudgetDonut({ windows }: { windows: { since: string; until: string } }) {
  const budgets = useBudgets();
  const cost = useCostRollup(windows.since, windows.until, "area");

  const caps = (budgets.data?.items ?? []).filter((b) => b.enabled && b.micros);
  const spentByArea = new Map(
    (cost.data?.buckets ?? []).map((b) => [b.key, b.cost.micros ?? 0]),
  );

  const rows = caps.map((budget) => {
    const key = budget.scope?.area ?? "";
    const spent = spentByArea.get(key) ?? 0;
    return { key, label: scopeLabel(budget), cap: budget.micros ?? 0, spent };
  });
  const cap = rows.reduce((sum, r) => sum + r.cap, 0);
  const spent = rows.reduce((sum, r) => sum + r.spent, 0);

  return (
    <section className="flex flex-col gap-4 rounded-xl border border-border bg-card p-4 shadow-sm">
      <h2 className="text-sm font-medium">Teto de custo · hoje</h2>

      {budgets.isLoading || cost.isLoading ? (
        <Skeleton className="h-32 rounded-lg" />
      ) : rows.length === 0 ? (
        <p className="py-6 text-sm text-muted-foreground">
          Nenhum teto por escopo configurado, então não há denominador para uma
          porcentagem. Gasto hoje: {formatCost({ micros: totalOf(spentByArea) })}.{" "}
          <Link to="/admin" className="text-primary hover:underline">
            Configurar
          </Link>
        </p>
      ) : (
        <div className="flex flex-wrap items-center gap-5">
          <Donut share={cap > 0 ? spent / cap : 0} cap={cap} />
          <ul className="flex min-w-[160px] flex-1 flex-col gap-2.5">
            {rows.map((row) => (
              <li key={row.key} className="flex flex-col gap-1">
                <div className="flex items-baseline justify-between gap-3">
                  <span className="truncate text-xs">{row.label}</span>
                  <Mono dim>
                    {formatCost({ micros: row.spent })} / {formatCost({ micros: row.cap })}
                  </Mono>
                </div>
                <Bar share={row.cap > 0 ? row.spent / row.cap : 0} />
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}

function totalOf(spent: Map<string, number>): number {
  return [...spent.values()].reduce((sum, v) => sum + v, 0);
}

const RADIUS = 46;
const CIRCUMFERENCE = 2 * Math.PI * RADIUS;

function Donut({ share, cap }: { share: number; cap: number }) {
  const filled = Math.min(share, 1);

  return (
    <div className="relative size-28 shrink-0">
      <svg viewBox="0 0 112 112" className="size-28 -rotate-90">
        <circle cx="56" cy="56" r={RADIUS} fill="none" strokeWidth="10" className="stroke-muted" />
        <circle
          cx="56"
          cy="56"
          r={RADIUS}
          fill="none"
          strokeWidth="10"
          strokeLinecap="round"
          strokeDasharray={`${filled * CIRCUMFERENCE} ${CIRCUMFERENCE}`}
          className={cn(share >= 0.75 ? "stroke-warning" : "stroke-primary")}
        />
      </svg>
      <div className="absolute inset-0 flex flex-col items-center justify-center">
        <span className="font-mono text-xl font-medium tabular-nums">
          {Math.round(share * 100)}%
        </span>
        <span className="text-2xs text-muted-foreground">de {formatCost({ micros: cap })}</span>
      </div>
    </div>
  );
}

function Bar({ share }: { share: number }) {
  return (
    <div className="h-[5px] overflow-hidden rounded-pill bg-muted">
      <span
        aria-hidden
        className={cn(
          "block h-full rounded-pill",
          share >= 0.75 ? "bg-warning" : "bg-primary",
        )}
        style={{ width: `${Math.min(share, 1) * 100}%` }}
      />
    </div>
  );
}
