import type { Cost, Step } from "@/lib/api/client";

/** Why a run's cost reads as nothing, when it does. */
export type UnpricedReason =
  | "missing_rate"
  | "partial_missing_rate"
  | "configured_zero"
  | "rounded_zero"
  | "nothing_spent"
  | "unknown";

export interface RunSpend {
  /** False when the figure on screen is not a complete non-zero price. */
  priced: boolean;
  reason?: UnpricedReason;
  /** Content bytes by source, summed across the run's planning turns. */
  bytes: Record<string, number>;
  /** The same bytes attributed to the tool that produced them, so a heavy
   *  prompt names a cause rather than a category. */
  byTool: Record<string, number>;
}

/**
 * The unit the platform measures prompt composition in.
 *
 * Checked rather than assumed: the payload names its unit so a later one
 * cannot be summed into this total by a reader that did not notice it changed.
 */
const CONTENT_BYTES = "content_bytes";

/**
 * The sources a prompt is composed of, named rather than inferred.
 *
 * An allow list rather than an exclusion list, because the first version of
 * this summed every number it did not recognise and swallowed `total` — which
 * doubled the composition and made the proportion bar divide against itself. A
 * field added to the payload later is now ignored until somebody adds it here,
 * which is the safe direction: a missing source is visibly absent, an invented
 * one is a chart that lies.
 */
const SOURCES = [
  "instructions",
  "platform",
  "input",
  "notes",
  "tool_schemas",
  "tool_arguments",
  "tool_results",
] as const;

/**
 * What a run spent, and whether the number means what it looks like.
 *
 * Zero cost has several different causes. A run that called nothing spent
 * nothing; a run without a configured rate is unpriced; a configured zero rate
 * is deliberate; and a non-zero rate can round below one micro. A positive
 * total can still be partial when one planning turn had a rate and another did
 * not. The ledger now records price provenance on each planned step, so this
 * screen reads rather than guesses. Old zero-cost runs without that field fall
 * back to "unknown".
 *
 * Bytes and tokens are returned side by side and never combined. Bytes are
 * measured by this platform while assembling the prompt; tokens are what the
 * provider reported and the only thing money is derived from. A component that
 * divided one by the other would be inventing a rate.
 */
export function runSpend(cost: Cost, steps: Step[]): RunSpend {
  const reason = priceReason(cost, steps);
  return {
    priced: reason === undefined,
    reason,
    bytes: promptBytes(steps),
    byTool: promptBytesByTool(steps),
  };
}

function priceReason(cost: Cost, steps: Step[]): UnpricedReason | undefined {
  const micros = cost.micros ?? 0;
  if (tokensOf(cost) === 0) return "nothing_spent";
  const uses = priceUses(steps);
  if (uses.length === 0) return micros > 0 ? undefined : "unknown";
  if (uses.some((use) => use.status === "missing")) {
    return micros > 0 ? "partial_missing_rate" : "missing_rate";
  }
  if (uses.some((use) => use.status !== "configured")) {
    return micros > 0 ? undefined : "unknown";
  }
  if (micros > 0) return undefined;
  if (uses.some((use) => use.non_zero_applied === true)) return "rounded_zero";
  return "configured_zero";
}

function tokensOf(cost: Cost): number {
  return (
    (cost.inputTokens ?? 0) +
    (cost.outputTokens ?? 0) +
    (cost.cacheReadTokens ?? 0) +
    (cost.cacheWriteTokens ?? 0)
  );
}

interface PriceUse {
  status?: string;
  non_zero_applied?: boolean;
}

function priceUses(steps: Step[]): PriceUse[] {
  const out: PriceUse[] = [];
  for (const step of steps) {
    const payload = (step.payload ?? {}) as Record<string, unknown>;
    const price = payload.price;
    if (price && typeof price === "object" && !Array.isArray(price)) {
      out.push(price as PriceUse);
    }
  }
  return out;
}

/** The per-tool maps, which say which tool made a prompt heavy. */
const BY_TOOL = ["tool_arguments_by_tool", "tool_results_by_tool"] as const;

function promptBytesByTool(steps: Step[]): Record<string, number> {
  const total: Record<string, number> = {};
  for (const prompt of compositions(steps)) {
    for (const field of BY_TOOL) {
      const attributed = prompt[field];
      if (typeof attributed !== "object" || attributed === null) continue;
      for (const [tool, value] of Object.entries(attributed)) {
        if (typeof value !== "number") continue;
        total[tool] = (total[tool] ?? 0) + value;
      }
    }
  }
  return total;
}

function* compositions(steps: Step[]) {
  for (const step of steps) {
    const payload = (step.payload ?? {}) as Record<string, unknown>;
    const prompt = payload.prompt as Record<string, unknown> | undefined;
    if (prompt && prompt.unit === CONTENT_BYTES) yield prompt;
  }
}

function promptBytes(steps: Step[]): Record<string, number> {
  const total: Record<string, number> = {};
  for (const prompt of compositions(steps)) {
    for (const source of SOURCES) {
      const value = prompt[source];
      if (typeof value !== "number") continue;
      total[source] = (total[source] ?? 0) + value;
    }
  }
  return total;
}
