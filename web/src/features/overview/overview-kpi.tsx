import { TrendingDown, TrendingUp } from "lucide-react";
import { Sparkline } from "@/components/shared/sparkline";
import { cn } from "@/lib/utils";

/**
 * One figure, what it is measured against, and how it got there.
 *
 * The delta is absent rather than zero when there is nothing to compare
 * against. "+100%" from a base of nothing is not a measurement, and printing
 * it as one is how a dashboard starts lying quietly.
 */
export function OverviewKpi({
  label,
  value,
  unit,
  note,
  delta,
  trend,
  tone = "neutral",
}: {
  label: string;
  value: string;
  unit?: string;
  note: string;
  delta?: number;
  trend?: number[];
  tone?: "neutral" | "bad";
}) {
  return (
    <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
      <div className="text-2xs uppercase tracking-label text-muted-foreground">{label}</div>

      <div className="mt-1.5 flex items-baseline gap-1.5">
        <span
          className={cn(
            "font-mono text-[26px]/8 font-medium tabular-nums",
            tone === "bad" && "text-danger",
          )}
        >
          {value}
        </span>
        {unit && <span className="text-sm text-muted-foreground">{unit}</span>}
      </div>

      <div className="mt-1 flex items-end justify-between gap-2">
        <div className="flex min-w-0 flex-col">
          {delta !== undefined && <Delta value={delta} />}
          <span className="truncate text-xs text-muted-foreground">{note}</span>
        </div>
        {trend && <Sparkline points={trend} className={deltaTone(delta)} />}
      </div>
    </div>
  );
}

function deltaTone(delta?: number): string {
  return delta !== undefined && delta < 0 ? "text-danger" : "text-primary";
}

function Delta({ value }: { value: number }) {
  const up = value >= 0;
  const Icon = up ? TrendingUp : TrendingDown;

  // Direction only. More runs is not inherently good and fewer is not
  // inherently bad — this is a volume, not a score.
  return (
    <span className={cn("flex items-center gap-1 text-xs", up ? "text-success" : "text-danger")}>
      <Icon className="size-3.5" aria-hidden />
      <span className="font-mono tabular-nums">
        {up ? "+" : ""}
        {Math.round(value * 100)}%
      </span>
    </span>
  );
}
