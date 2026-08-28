import type { MemoryAssertion, MemoryStatus } from "@/features/memory/api";

/**
 * What the platform already holding this identity means, and what may be done
 * about it.
 *
 * A table rather than a chain of conditions in the component, because the four
 * states differ in what they *offer* and not only in what they say — and a
 * state that quietly falls through to the default would offer an act the
 * platform will refuse. Every status names itself here; there is no else.
 */
export interface MatchState {
  /** What is true, in one sentence. */
  says: string;
  /** What saving this form would do to it, when saving is still a sensible
   *  act. Absent means the memory cannot be written to at all. */
  saving?: string;
  /** Whether the memory can be brought back, which is its own act. */
  reactivable?: boolean;
}

export const OWN_STATE: Record<MemoryStatus, MatchState> = {
  // Correcting, not duplicating: the merge finds the same identity and rewrites
  // the claim, keeping the counters, the authorship and the evidence.
  active: { says: "memory.matchActive", saving: "memory.matchActiveSaving" },

  // The server refuses to merge into a disabled row, so saving is not the way
  // back — offering it would be offering a refusal. Reactivation is.
  disabled: { says: "memory.matchDisabled", reactivable: true },

  // Still here, and out of the reach of recall until it is reasserted. Saving
  // is exactly the act that renews it, so there is nothing else to offer.
  expired: { says: "memory.matchExpired", saving: "memory.matchExpiredSaving" },

  // The run this rested on was erased. Nothing brings it back and nothing
  // should: the record of the erasure is the point. Shown so the person
  // understands why teaching this again starts from nothing.
  source_erased: { says: "memory.matchErased" },
};

/** Whether the shared memory covering an agent-scoped question can be improved
 *  rather than shadowed. Only an active one can: the rest are states of that
 *  shared row, and acting on them belongs to whoever owns it. */
export function sharedIsImprovable(shared: MemoryAssertion): boolean {
  return shared.status === "active";
}
