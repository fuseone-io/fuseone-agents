import { TrendingDown, TrendingUp } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * One figure, its comparison, and nothing else.
 *
 * The delta is absent rather than zero when there is nothing to compare
 * against. "+100%" from a base of nothing is not a measurement, and printing
 * it as one is how a dashboard starts lying quietly.
 */
export function OverviewKpi({
  label,
  value,
  note,
  delta,
  tone = "neutral",
}: {
  label: string;
  value: string;
  note: string;
  delta?: number;
  tone?: "neutral" | "bad";
}) {
  return (
    <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
      <div className="text-2xs uppercase tracking-label text-muted-foreground">{label}</div>
      <div className="mt-1.5 flex items-baseline gap-2">
        <span
          className={cn(
            "font-mono text-[26px]/8 font-medium tabular-nums",
            tone === "bad" && "text-danger",
          )}
        >
          {value}
        </span>
        {delta !== undefined && <Delta value={delta} />}
      </div>
      <div className="mt-0.5 text-xs text-muted-foreground">{note}</div>
    </div>
  );
}

function Delta({ value }: { value: number }) {
  const up = value >= 0;
  const Icon = up ? TrendingUp : TrendingDown;

  // More runs is not inherently good and fewer is not inherently bad — this
  // is a volume, not a score — so the colour reports direction only.
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
