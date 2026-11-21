import type { components } from "@/lib/api/schema.gen";

export type ReplayDivergence = components["schemas"]["ReplayDivergence"];

/**
 * The words for how a replayed case left the world it was recorded with (NT-018 phase 5).
 * Written out rather than built from the value, so the guard that checks every key exists
 * can see them.
 */
const DIVERGED: Record<ReplayDivergence["divergence"], string> = {
  missing: "runs.storyReplayDiverged_missing",
  extra: "runs.storyReplayDiverged_extra",
  repeated: "runs.storyReplayDiverged_repeated",
  out_of_order: "runs.storyReplayDiverged_out_of_order",
};

export function divergedKey(d: ReplayDivergence): string {
  return DIVERGED[d.divergence];
}
