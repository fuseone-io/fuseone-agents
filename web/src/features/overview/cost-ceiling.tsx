import { Skeleton } from "@/components/ui/skeleton";
import { Mono } from "@/components/shared/mono";
import { useCostRollup } from "@/features/cost/api";
import { formatCost } from "@/lib/format";
import type { CostBucket } from "@/lib/api/client";

/**
 * Where today's spend went, by area.
 *
 * Deliberately not a share of a ceiling: budgets are set per scope and a run
 * can be governed by a company limit, an area limit, or neither, so a single
 * "32% of the cap" would be a figure with no defined denominator. Until this
 * panel can name which ceiling it is measuring against, it reports what was
 * spent and by whom.
 */
export function CostCeiling({ windows }: { windows: { since: string; until: string } }) {
  const { data, isLoading, error } = useCostRollup(windows.since, windows.until, "area");

  const buckets = [...(data?.buckets ?? [])].sort(
    (a, b) => (b.cost.micros ?? 0) - (a.cost.micros ?? 0),
  );
  const total = buckets.reduce((sum, b) => sum + (b.cost.micros ?? 0), 0);

  return (
    <section className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 shadow-sm">
      <h2 className="text-sm font-medium">Custo por área · hoje</h2>

      {isLoading ? (
        <Skeleton className="h-32 rounded-lg" />
      ) : error ? (
        <p className="py-8 text-center text-sm text-muted-foreground">
          Não foi possível ler o custo de hoje.
        </p>
      ) : buckets.length === 0 ? (
        <p className="py-8 text-center text-sm text-muted-foreground">
          Nada consumido hoje.
        </p>
      ) : (
        <>
          <div className="font-mono text-[26px]/8 font-medium tabular-nums">
            {formatCost({ micros: total })}
          </div>
          <ul className="flex flex-col gap-2.5">
            {buckets.map((bucket) => (
              <Share key={bucket.key} bucket={bucket} total={total} />
            ))}
          </ul>
        </>
      )}
    </section>
  );
}

function Share({ bucket, total }: { bucket: CostBucket; total: number }) {
  const micros = bucket.cost.micros ?? 0;
  const share = total > 0 ? micros / total : 0;

  return (
    <li className="flex flex-col gap-1">
      <div className="flex items-baseline justify-between gap-3">
        <span className="truncate text-xs">{bucket.key || "sem área"}</span>
        <Mono dim className="shrink-0">
          {formatCost(bucket.cost)} · {bucket.runs} exec.
        </Mono>
      </div>
      <div className="h-[5px] overflow-hidden rounded-pill bg-muted">
        <span
          aria-hidden
          className="block h-full rounded-pill bg-primary"
          style={{ width: `${Math.round(share * 100)}%` }}
        />
      </div>
    </li>
  );
}
