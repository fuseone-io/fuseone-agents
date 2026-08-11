import { cn } from "@/lib/utils";

export interface Bar {
  /** What the bar is, for the axis and the accessible description. */
  label: string;
  value: number;
  /** Rendered in the tooltip and the description, already formatted. */
  display: string;
}

/**
 * A bar chart that reads as a chart to a mouse and as a sentence to a screen
 * reader.
 *
 * Deterministic by construction: no animation, no measured layout, so the same
 * data draws the same picture — which matters because a figure in a governance
 * console gets screenshotted into tickets.
 */
export function BarChart({ bars, label, className }: { bars: Bar[]; label: string; className?: string }) {
  const max = Math.max(...bars.map((b) => b.value), 1);
  const width = 620;
  const height = 150;
  const baseline = 128;
  const slot = width / Math.max(bars.length, 1);

  return (
    <figure className={cn("m-0", className)}>
      <svg
        viewBox={`0 0 ${width} ${height}`}
        preserveAspectRatio="none"
        className="block h-[150px] w-full"
        role="img"
        aria-label={label}
      >
        {bars.map((bar, i) => {
          const barHeight = (bar.value / max) * (baseline - 10);
          return (
            <rect
              key={bar.label}
              x={i * slot + slot * 0.2}
              y={baseline - barHeight}
              width={slot * 0.6}
              height={barHeight}
              rx={2}
              // The most recent column is the one being asked about; the rest
              // are context for it.
              className={i === bars.length - 1 ? "fill-primary" : "fill-[var(--fuse-200)]"}
            >
              <title>
                {bar.label}: {bar.display}
              </title>
            </rect>
          );
        })}
        <line x1={0} x2={width} y1={baseline} y2={baseline} className="stroke-border" strokeWidth={1} />
      </svg>

      {/* The same data as text, for anyone who cannot see the bars. */}
      <figcaption className="sr-only">
        {bars.map((bar) => `${bar.label}: ${bar.display}`).join(", ")}
      </figcaption>
    </figure>
  );
}
