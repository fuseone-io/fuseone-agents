import type { ThroughputBucket } from "@/lib/api/client";
import { stateOfPhase, type AgentState } from "@/lib/agent-state";

/**
 * The chart's data, prepared away from the drawing.
 *
 * The store reports only hours that had runs, because it answers what it
 * measured. A chart needs every hour in the period, or a quiet morning reads
 * as a narrower day rather than an emptier one.
 */

/** The three columns a bar stacks, in the order they stack. */
export const SEGMENTS: AgentState[] = ["done", "waiting", "blocked"];

export interface Column {
  /** The hour, 0–23, as it appears on the axis. */
  hour: number;
  at: string;
  byState: Record<AgentState, number>;
  total: number;
  micros: number;
}

export function columnsFor(buckets: ThroughputBucket[], since: string, hours = 24): Column[] {
  const start = new Date(since);
  const byHour = new Map<number, ThroughputBucket>();
  for (const bucket of buckets) {
    byHour.set(hoursBetween(start, new Date(bucket.at)), bucket);
  }

  return Array.from({ length: hours }, (_, i) => {
    const at = new Date(start.getTime() + i * 3_600_000);
    return columnOf(at, byHour.get(i));
  });
}

function columnOf(at: Date, bucket: ThroughputBucket | undefined): Column {
  const byState: Record<AgentState, number> = {
    draft: 0, running: 0, waiting: 0, blocked: 0, done: 0,
  };
  for (const [phase, count] of Object.entries(bucket?.byPhase ?? {})) {
    byState[stateOfPhase(phase as never)] += count;
  }
  // Running folds into waiting rather than getting a fourth column: the chart
  // separates finished, still going, and stopped — a reader deciding whether
  // to act does not need the interpreter's phase.
  byState.waiting += byState.running;
  byState.running = 0;

  return {
    hour: at.getHours(),
    at: at.toISOString(),
    byState,
    total: bucket?.total ?? 0,
    micros: bucket?.micros ?? 0,
  };
}

function hoursBetween(from: Date, to: Date): number {
  return Math.round((to.getTime() - from.getTime()) / 3_600_000);
}

/**
 * The top of the axis, rounded up so the tallest bar does not touch it.
 *
 * Never zero: an empty day still needs gridlines, or the panel renders as a
 * blank rectangle that reads as broken rather than quiet.
 */
export function ceilingOf(columns: Column[], step = 4): number {
  const tallest = Math.max(0, ...columns.map((c) => c.total));
  return Math.max(step, Math.ceil(tallest / step) * step);
}
