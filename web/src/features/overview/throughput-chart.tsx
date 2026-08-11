import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { SEGMENTS, type Column } from "@/features/overview/throughput-model";
import { STATE_DOT } from "@/lib/agent-state";
import { cn } from "@/lib/utils";

const GRIDLINES = 4;

/**
 * The bars themselves, drawn with layout rather than SVG.
 *
 * A grid of divs because every bar is a hover target with a popover and a
 * label, and reimplementing focus and keyboard reach inside an SVG is how
 * charts become unreachable by anything but a mouse.
 */
export function ThroughputChart({ columns, ceiling }: { columns: Column[]; ceiling: number }) {
  return (
    <div className="flex gap-2">
      <div className="flex h-[200px] w-8 flex-col justify-between pb-5 text-right">
        {Array.from({ length: GRIDLINES + 1 }, (_, i) => (
          <span key={i} className="font-mono text-2xs tabular-nums text-muted-foreground">
            {Math.round((ceiling * (GRIDLINES - i)) / GRIDLINES)}
          </span>
        ))}
      </div>

      <div className="relative min-w-0 flex-1">
        <div aria-hidden className="absolute inset-x-0 top-0 flex h-[180px] flex-col justify-between">
          {Array.from({ length: GRIDLINES + 1 }, (_, i) => (
            <span
              key={i}
              className={cn("h-px w-full", i === GRIDLINES ? "bg-border" : "bg-border-subtle")}
            />
          ))}
        </div>

        <ol className="relative flex h-[200px] items-end gap-px">
          {columns.map((column) => (
            <Bar key={column.at} column={column} ceiling={ceiling} />
          ))}
        </ol>
      </div>
    </div>
  );
}

function Bar({ column, ceiling }: { column: Column; ceiling: number }) {
  return (
    <li className="flex h-full min-w-0 flex-1 flex-col justify-end">
      <Tooltip>
        <TooltipTrigger className="group flex h-[180px] flex-col justify-end rounded-t-sm focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/26">
          <span className="sr-only">
            {column.hour}h: {column.total} execuções
          </span>
          <span className="flex w-full flex-col justify-end px-[28%]">
            {SEGMENTS.map((state, i) => (
              <span
                key={state}
                aria-hidden
                className={cn(
                  "w-full transition-opacity",
                  STATE_DOT[state],
                  i === 0 && "rounded-t-[3px]",
                )}
                style={{ height: `${(column.byState[state] / ceiling) * 180}px` }}
              />
            ))}
          </span>
        </TooltipTrigger>
        <TooltipContent>
          <p className="font-mono text-2xs tabular-nums">
            {String(column.hour).padStart(2, "0")}:00 · {column.total}
          </p>
        </TooltipContent>
      </Tooltip>

      {/* Every third hour, or the axis becomes a smear at console widths. */}
      <span className="h-5 pt-1 text-center font-mono text-2xs tabular-nums text-muted-foreground">
        {column.hour % 3 === 0 ? column.hour : ""}
      </span>
    </li>
  );
}
