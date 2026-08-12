import { Handle, Position, type NodeProps } from "@xyflow/react";
import {
  Flag, Hand, Play, ShieldCheck, Sparkles, TriangleAlert, Wrench, Zap,
  type LucideIcon,
} from "lucide-react";
import { Mono } from "@/components/shared/mono";
import { TILE } from "@/features/runs/trail-icon";
import { NODE_HEIGHT, NODE_WIDTH } from "@/features/runs/run-graph-layout";
import type { FlowKind, FlowNode as Model } from "@/features/runs/run-graph";
import { cn } from "@/lib/utils";

const ICONS: Record<FlowKind, LucideIcon> = {
  trigger: Play,
  agent: Sparkles,
  policy: ShieldCheck,
  tool: Wrench,
  // A call that changes a real system does not share an icon with a lookup.
  action: Zap,
  human: Hand,
  seal: Flag,
  fault: TriangleAlert,
};

/** Latency reads in the unit a person would say it in. */
function elapsed(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  if (ms < 3_600_000) return `${Math.round(ms / 60_000)}min`;
  return `${(ms / 3_600_000).toFixed(1)}h`;
}

export function FlowNode({ data }: NodeProps & { data: Model }) {
  const Icon = ICONS[data.kind];

  return (
    <div
      className={cn(
        "flex flex-col gap-1 rounded-xl border bg-card p-3 text-left shadow-sm",
        TILE[data.tone],
      )}
      style={{ width: NODE_WIDTH, height: NODE_HEIGHT }}
    >
      <Anchors />

      <div className="flex items-center gap-2">
        <Icon className="size-3.5 shrink-0" aria-hidden />
        <span className="truncate text-xs font-medium text-foreground">{data.title}</span>
      </div>

      <div className="flex items-baseline justify-between gap-2">
        <Mono dim className="truncate text-2xs">
          {data.detail ?? ""}
        </Mono>
        {/* Nothing where a pair never closed. A zero would read as an instant
            answer, which is the opposite of a call still in flight. */}
        <Mono dim className="shrink-0 text-2xs">
          {data.latencyMs === undefined ? "—" : elapsed(data.latencyMs)}
        </Mono>
      </div>
    </div>
  );
}

const SIDES = [
  ["left", Position.Left],
  ["right", Position.Right],
  ["top", Position.Top],
  ["bottom", Position.Bottom],
] as const;

/**
 * One anchor per side, as both an end and a start.
 *
 * Four sides because the layout is a serpentine: every other row reads
 * backwards, and an edge anchored right-to-left regardless exits the node,
 * loops around the outside of the canvas and comes back. Invisible, since
 * nothing here connects to anything — the picture is of something that already
 * happened.
 */
function Anchors() {
  return (
    <>
      {SIDES.map(([side, position]) => (
        <div key={side}>
          <Handle id={side} type="target" position={position} className="!opacity-0" />
          <Handle id={`${side}-out`} type="source" position={position} className="!opacity-0" />
        </div>
      ))}
    </>
  );
}
