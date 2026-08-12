import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export type Trend = "up" | "down" | "flat";

const TREND: Record<Trend, string> = {
  up: "text-success",
  down: "text-danger",
  flat: "text-muted-foreground",
};

/**
 * One number, stated plainly.
 *
 * The delta line is a sentence with a number in it — "32% of a $150 cap" —
 * not a bare arrow. A figure without its comparison is the kind of dashboard
 * nobody can act on.
 */
export function KpiCard({
  label,
  value,
  unit,
  delta,
  trend = "flat",
  children,
}: {
  label: string;
  value: string;
  unit?: string;
  delta?: string;
  trend?: Trend;
  /** Sparkline or other trailing visual. */
  children?: ReactNode;
}) {
  return (
    <div className="min-w-0 flex-1 rounded-xl border bg-card p-[18px] shadow-sm">
      <div className="text-2xs uppercase tracking-label text-muted-foreground">
        {label}
      </div>

      <div className="mt-2 flex items-baseline gap-1">
        <span className="font-mono text-[26px] font-medium leading-none tabular-nums">
          {value}
        </span>
        {unit && <span className="text-sm text-muted-foreground">{unit}</span>}
      </div>

      <div className="mt-1 flex items-end gap-2.5">
        <div className={cn("flex-1 text-xs", TREND[trend])}>{delta}</div>
        {children}
      </div>
    </div>
  );
}
