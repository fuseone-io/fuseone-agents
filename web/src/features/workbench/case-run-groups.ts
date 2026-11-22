import type { ExperimentCaseChange } from "@/features/experiments/experiments-api";

/** How a case moved from the published version to the candidate. */
export type CaseGroup = "regressed" | "fixed" | "kept" | "apart";

export const GROUPS: readonly CaseGroup[] = ["regressed", "fixed", "kept", "apart"];

/**
 * Held to broke is a regression, broke to held a fix, held to held kept (NT-018 phase 5c).
 * Anything else, a case still broken or one that did not run or cannot be compared on a
 * side, is set apart: it says nothing about which version is better.
 */
export function groupOf(change: ExperimentCaseChange): CaseGroup {
  if (change.was === "held" && change.now === "broke") return "regressed";
  if (change.was === "broke" && change.now === "held") return "fixed";
  if (change.was === "held" && change.now === "held") return "kept";
  return "apart";
}

export function grouped(changes: ExperimentCaseChange[]): Record<CaseGroup, ExperimentCaseChange[]> {
  const out: Record<CaseGroup, ExperimentCaseChange[]> = { regressed: [], fixed: [], kept: [], apart: [] };
  for (const change of changes) out[groupOf(change)].push(change);
  return out;
}
