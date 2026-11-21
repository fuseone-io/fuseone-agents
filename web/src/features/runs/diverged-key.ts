import type { components } from "@/lib/api/schema.gen";

export type ReplayDivergence = components["schemas"]["ReplayDivergence"];

/** The words for how a replayed case left the world it was recorded with (NT-018 phase 5). */
export function divergedKey(d: ReplayDivergence): string {
  return `runs.storyReplayDiverged_${d.divergence}`;
}
