import { useRuns } from "@/features/runs/api";
import type { Run } from "@/lib/api/client";

/**
 * Which run the trace should open on when nobody has chosen one.
 *
 * The panel exists so somebody can act, so it opens on a run waiting for a
 * person before one that merely finished. If nothing is waiting, the most
 * recent says what just happened, which is the next most useful thing a
 * dashboard can show without being asked. If nothing ran, it opens on
 * nothing: an empty panel taking a third of the width to say so is worse than
 * no panel.
 */
export function suggestedRun(runs: Run[]): string | undefined {
  const waiting = runs.find((run) => run.phase === "awaiting_approval");
  return (waiting ?? runs[0])?.runId;
}

export function useSuggestedRun(since: string): string | undefined {
  const { items } = useRuns({ since });
  return suggestedRun(items);
}
