import type { Cost, Step } from "@/lib/api/client";

/** Why a run's cost reads as nothing, when it does. */
export type UnpricedReason = "no_rate" | "nothing_spent";

export interface RunSpend {
  /** False when the figure on screen is not a price anybody set. */
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
    byTool: promptBytesByTool(steps),
  };
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
