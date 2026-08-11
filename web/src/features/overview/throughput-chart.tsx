import { useState } from "react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { SEGMENTS, type Column } from "@/features/overview/throughput-model";
import { STATE_DOT } from "@/lib/agent-state";
import { formatCost } from "@/lib/format";
import { cn } from "@/lib/utils";

const PLOT = 180;
const AXIS = 20;
const GRIDLINES = 4;
const LABEL: Record<(typeof SEGMENTS)[number], string> = {
  done: "concluídas",
  waiting: "em curso",
  blocked: "paradas",
};

/**
 * The day's runs, stacked by what became of them.
 *
 * The bars are laid out rather than drawn in SVG, so each one is a real focus
 * target with a popover instead of a shape a mouse can hover. The area behind
 * them — the hourly total — is SVG, because it is one continuous curve and
 * nothing about it is interactive.
 */
export function ThroughputChart({ columns, ceiling }: { columns: Column[]; ceiling: number }) {
  const [hovered, setHovered] = useState<string | null>(null);

  return (
    <div className="flex gap-2">
      <div className="flex w-8 flex-col justify-between text-right" style={{ height: PLOT }}>
        {Array.from({ length: GRIDLINES + 1 }, (_, i) => (
          <span key={i} className="font-mono text-2xs tabular-nums text-muted-foreground">
            {Math.round((ceiling * (GRIDLINES - i)) / GRIDLINES)}
          </span>
        ))}
      </div>

      <div className="relative min-w-0 flex-1">
        <Grid />
        <Area columns={columns} ceiling={ceiling} />

        <ol className="relative flex items-end gap-px" style={{ height: PLOT + AXIS }}>
          {columns.map((column) => (
            <Bar
              key={column.at}
              column={column}
              ceiling={ceiling}
              dimmed={hovered !== null && hovered !== column.at}
              onHover={setHovered}
            />
          ))}
        </ol>
      </div>
    </div>
  );
}

/** Dashed except the baseline, which is the axis rather than a reading aid. */
function Grid() {
  return (
    <div
      aria-hidden
      className="absolute inset-x-0 top-0 flex flex-col justify-between"
      style={{ height: PLOT }}
    >
      {Array.from({ length: GRIDLINES + 1 }, (_, i) => (
        <span
          key={i}
          className={cn(
            "h-px w-full",
            i === GRIDLINES
              ? "bg-border"
              : "bg-[repeating-linear-gradient(to_right,var(--border-subtle)_0_2px,transparent_2px_6px)]",
          )}
        />
      ))}
    </div>
  );
}

/** The hourly total traced behind the bars, so the shape of the day survives
 *  a stack that changes colour from column to column. */
function Area({ columns, ceiling }: { columns: Column[]; ceiling: number }) {
  if (columns.length < 2) return null;

  const points = columns.map((c, i) => {
    const x = (i / (columns.length - 1)) * 100;
    const y = PLOT - (c.total / ceiling) * PLOT;
    return `${x.toFixed(2)},${y.toFixed(1)}`;
  });

  return (
    <svg
      aria-hidden
      className="pointer-events-none absolute inset-x-0 top-0 text-primary"
      viewBox={`0 0 100 ${PLOT}`}
      preserveAspectRatio="none"
      style={{ height: PLOT }}
    >
      <defs>
        <linearGradient id="throughput-area" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="currentColor" stopOpacity="0.14" />
          <stop offset="100%" stopColor="currentColor" stopOpacity="0" />
        </linearGradient>
      </defs>
      <polygon points={`0,${PLOT} ${points.join(" ")} 100,${PLOT}`} fill="url(#throughput-area)" />
    </svg>
  );
}

function Bar({
  column,
  ceiling,
  dimmed,
  onHover,
}: {
  column: Column;
  ceiling: number;
  dimmed: boolean;
  onHover: (at: string | null) => void;
}) {
  return (
    <li className="flex h-full min-w-0 flex-1 flex-col justify-end">
      <Tooltip>
        <TooltipTrigger
          className="flex flex-col justify-end rounded-t-sm focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/26"
          style={{ height: PLOT }}
          onMouseEnter={() => onHover(column.at)}
          onMouseLeave={() => onHover(null)}
          onFocus={() => onHover(column.at)}
          onBlur={() => onHover(null)}
        >
          <span className="sr-only">
            {column.hour}h: {column.total} execuções
          </span>
          <span
            className={cn(
              "flex w-full flex-col justify-end px-[28%] transition-opacity",
              dimmed && "opacity-35",
            )}
          >
            {SEGMENTS.map((state, i) => (
              <span
                key={state}
                aria-hidden
                className={cn("w-full", STATE_DOT[state], i === 0 && "rounded-t-[3px]")}
                style={{ height: `${(column.byState[state] / ceiling) * PLOT}px` }}
              />
            ))}
          </span>
        </TooltipTrigger>

        {/* The breakdown, not just the total: the reason to look at one hour
            is to find out which column of it grew. */}
        <TooltipContent className="flex flex-col gap-1">
          <p className="font-mono text-2xs tabular-nums">
            {String(column.hour).padStart(2, "0")}:00 – {String(column.hour + 1).padStart(2, "0")}:00
            {column.micros > 0 && ` · ${formatCost({ micros: column.micros })}`}
          </p>
          {SEGMENTS.map((state) => (
            <p key={state} className="flex items-center gap-2 text-2xs">
              <span aria-hidden className={cn("size-1.5 rounded-[2px]", STATE_DOT[state])} />
              {LABEL[state]}
              <span className="ml-auto font-mono tabular-nums">{column.byState[state]}</span>
            </p>
          ))}
        </TooltipContent>
      </Tooltip>

      {/* Every third hour, or the axis becomes a smear at console widths. */}
      <span
        className="pt-1 text-center font-mono text-2xs tabular-nums text-muted-foreground"
        style={{ height: AXIS }}
      >
        {column.hour % 3 === 0 ? column.hour : ""}
      </span>
    </li>
  );
}
