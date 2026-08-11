import type { Agent } from "@/lib/api/client";

/**
 * A share only where there is something to take a share of.
 *
 * "0%" on an agent that never ran is a measurement of nothing, and a reader
 * would take it for a failing agent rather than an idle one.
 */
export function successRate(agent: Agent): string {
  const activity = agent.activity;
  if (!activity || activity.runs === 0) return "—";
  return `${Math.round((activity.finished / activity.runs) * 100)}%`;
}

/** What each run cost on average, or nothing when none has run. */
export function costPerRun(agent: Agent): number | undefined {
  const activity = agent.activity;
  if (!activity || activity.runs === 0) return undefined;
  return Math.round(activity.costMicros / activity.runs);
}
