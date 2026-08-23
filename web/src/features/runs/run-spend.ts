import type { Cost, Step } from "@/lib/api/client";

/** Why a run's cost reads as nothing, when it does. */
export type UnpricedReason = "no_rate" | "nothing_spent";

export interface RunSpend {
  /** False when the figure on screen is not a price anybody set. */
  priced: boolean;
  reason?: UnpricedReason;
  /** Content bytes by source, summed across the run's planning turns. */
  bytes: Record<string, number>;
}

/**
 * The unit the platform measures prompt composition in.
 *
 * Checked rather than assumed: the payload names its unit so a later one
 * cannot be summed into this total by a reader that did not notice it changed.
 */
const CONTENT_BYTES = "content_bytes";

/** Fields of the composition payload that are not measurements. */
const NOT_A_SOURCE = new Set(["unit", "tools"]);

/**
 * What a run spent, and whether the number means what it looks like.
 *
 * Zero cost has two very different causes. A run that called nothing spent
 * nothing; a run that called a model nobody configured a rate for also reads
 * zero, and only one of those is a price. Reporting the figure without the
 * reason leaves an operator unable to tell a cheap agent from an unpriced one
 * — which is the confusion that made market defaults reference-only.
 *
 * Bytes and tokens are returned side by side and never combined. Bytes are
 * measured by this platform while assembling the prompt; tokens are what the
 * provider reported and the only thing money is derived from. A component that
 * divided one by the other would be inventing a rate.
 */
export function runSpend(cost: Cost, steps: Step[]): RunSpend {
  const tokens =
    (cost.inputTokens ?? 0) +
    (cost.outputTokens ?? 0) +
    (cost.cacheReadTokens ?? 0) +
    (cost.cacheWriteTokens ?? 0);

  const priced = (cost.micros ?? 0) > 0;
  return {
    priced,
    reason: priced ? undefined : tokens > 0 ? "no_rate" : "nothing_spent",
    bytes: promptBytes(steps),
  };
}

function promptBytes(steps: Step[]): Record<string, number> {
  const total: Record<string, number> = {};
  for (const step of steps) {
    const payload = (step.payload ?? {}) as Record<string, unknown>;
    const prompt = payload.prompt as Record<string, unknown> | undefined;
    if (!prompt || prompt.unit !== CONTENT_BYTES) continue;

    for (const [source, value] of Object.entries(prompt)) {
      if (NOT_A_SOURCE.has(source) || typeof value !== "number") continue;
      total[source] = (total[source] ?? 0) + value;
    }
  }
  return total;
}
