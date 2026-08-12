/**
 * Why a case stopped, in words.
 *
 * The ledger records a stable code so the trail stays machine-readable for
 * years; a screen showing that code makes the reader learn the platform's
 * vocabulary to find out what happened to their agent.
 *
 * An unknown code is shown as it came rather than hidden. A reason nobody
 * translated yet is still the most useful thing on the row, and a version of
 * the console older than the worker writing the trail is the normal state
 * during an upgrade.
 */
const REASONS: Record<string, string> = {
  no_progress: "simulation.reasonNoProgress",
  budget_exhausted: "simulation.reasonBudget",
  spec_unresolved: "simulation.reasonSpec",
  attempts_exhausted: "simulation.reasonAttempts",
  budget_unreadable: "simulation.reasonBudgetUnreadable",
};

export function reasonKey(reason: string): string | undefined {
  return REASONS[reason];
}
