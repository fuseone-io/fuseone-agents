import { Handle, Position } from "@xyflow/react";
import { useTranslation } from "react-i18next";
import { CircleAlert, Wrench } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import { cn } from "@/lib/utils";

const PORTS = [
  ["left", Position.Left],
  ["right", Position.Right],
  ["top", Position.Top],
  ["bottom", Position.Bottom],
] as const;

export interface StepNodeData {
  name: string;
  reaches: string[];
  stopsWhen?: string;
  index: number;
  [key: string]: unknown;
}

/**
 * One stage, as a card.
 *
 * A single row and no bands, per the handoff: the tile carries the kind, the
 * title carries the author's words, and what it may reach is mono because it
 * is machine-generated text somebody will paste into a search.
 *
 * The subtitle is what a step is *allowed* to call, not what it will call. A
 * step that reaches nothing is the agent thinking and says so, because an
 * empty line would read as a card somebody forgot to finish.
 */
export function StepNode({
  data,
  selected,
}: {
  data: StepNodeData;
  selected?: boolean;
}) {
  const { t } = useTranslation();

  return (
    <div
      className={cn(
        "flex h-[52px] w-[216px] items-center gap-2.5 rounded-md border bg-card px-2.5 shadow-xs transition-shadow",
        selected
          ? "border-primary shadow-md ring-4 ring-surface-accent"
          : "border-[color-mix(in_oklab,var(--primary)_55%,var(--border))]",
      )}
    >
      {/* Four ports, so an edge crossing a row wrap can leave the bottom and
          arrive at the top rather than doubling back across the canvas.

          All of them invisible, and that is the point rather than a shortcut:
          a visible handle is an invitation to draw a connection, and there is
          no connection to draw. The order is the sequence, and the sequence is
          changed by moving a card. */}
      {PORTS.map(([id, position]) => (
        <Handle
          key={`t-${id}`}
          id={`t-${id}`}
          type="target"
          position={position}
          className="!opacity-0"
        />
      ))}
      <span className="grid size-[30px] shrink-0 place-items-center rounded-md border border-primary/25 bg-primary/10 text-2xs tabular-nums text-primary">
        {data.index + 1}
      </span>
      <div className="min-w-0 flex-1">
        {/* A card with a blank title is one nobody can identify later, and
            the tool is not its name: two stages can call the same one. */}
        <p
          className={cn(
            "truncate text-xs font-medium",
            !data.name && "italic text-muted-foreground",
          )}
        >
          {data.name || t("agents.unnamedStep")}
        </p>
        <p className="flex items-center gap-1 truncate text-[11px] text-muted-foreground">
          {data.reaches.length > 0 ? (
            <>
              <Wrench className="size-3 shrink-0" aria-hidden />
              <Mono className="truncate text-[11px]">
                {data.reaches.join(", ")}
              </Mono>
            </>
          ) : (
            t("agents.thinksOnly")
          )}
        </p>
      </div>
      {data.stopsWhen && (
        <CircleAlert
          className="size-3.5 shrink-0 text-muted-foreground"
          aria-label={data.stopsWhen}
        />
      )}
      {PORTS.map(([id, position]) => (
        <Handle
          key={`s-${id}`}
          id={`s-${id}`}
          type="source"
          position={position}
          className="!opacity-0"
        />
      ))}
    </div>
  );
}
