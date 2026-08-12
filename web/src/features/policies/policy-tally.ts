import type { Policy } from "@/lib/api/client";

/**
 * What the figures at the top of the screen are counted from.
 *
 * Every one is derivable exactly from what the API already sends. A policy has
 * one effect, so the decisions it produced are decisions of that kind — no
 * separate count is needed and none is invented.
 */
export interface Tally {
  enforcing: number;
  monitoring: number;
  disabled: number;
  hits: number;
  denied: number;
  escalated: number;
}

export function tallyOf(policies: Policy[]): Tally {
  const tally: Tally = {
    enforcing: 0, monitoring: 0, disabled: 0, hits: 0, denied: 0, escalated: 0,
  };

  for (const policy of policies) {
    if (policy.enabled === false) {
      tally.disabled += 1;
    } else if (policy.mode === "monitor") {
      tally.monitoring += 1;
    } else {
      tally.enforcing += 1;
    }

    const hits = policy.hits ?? 0;
    tally.hits += hits;
    if (policy.effect === "deny") tally.denied += hits;
    if (policy.effect === "escalate") tally.escalated += hits;
  }
  return tally;
}
