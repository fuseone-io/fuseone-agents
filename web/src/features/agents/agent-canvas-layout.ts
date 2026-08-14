import type { Cell } from "@/features/runs/run-graph-layout";

/**
 * The grid an agent's stages wrap into.
 *
 * The handoff's numbers, and the run diagram's serpentine — one implementation
 * of the wrap, two sets of dimensions. A second copy would drift the first
 * time either changed, and the two diagrams disagreeing about how a sequence
 * reads is exactly the confusion a shared shape prevents.
 */
export const STEP_CELL: Cell = {
  width: 216,
  height: 52,
  gapX: 64,
  gapY: 68,
  perRow: 4,
};
