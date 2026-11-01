import type { TFunction } from "i18next";
import type { components } from "@/lib/api/schema.gen";
import type { Locale } from "@/i18n/locale";

export type EventCycleWarning = components["schemas"]["EventCycleWarning"];

/**
 * What a publish that closed an event cycle says to its author (#227).
 *
 * Each cycle is written as the loop it is — back to the agent it started
 * from — because "developer, reviewer" reads as a list of agents, and the
 * thing worth seeing is that the last one starts the first again. The order
 * is the server's: it is stable, so two publishes can be compared by eye.
 *
 * A cycle of one gets its own sentence. The platform never starts an agent
 * from its own event, so telling that author the chain stops after N hops
 * would describe something that never happens.
 */
export function eventCycleMessage(
  warning: EventCycleWarning | undefined,
  t: TFunction,
  locale: Locale,
): string | undefined {
  if (!warning) return undefined;
  // Truncated with nothing named: the search ran out before it could tell,
  // which is not the same as finding no cycle.
  if (warning.cycles.length === 0) {
    return warning.truncated
      ? t("agents.eventCycleUnchecked", { hops: warning.hopLimit })
      : undefined;
  }
  const loops = warning.cycles.map((agents) =>
    [...agents, agents[0]].join(" → "),
  );
  const cycles = new Intl.ListFormat(locale, { type: "conjunction" }).format(
    loops,
  );
  const sentences = [
    t("agents.eventCycleWarning", { cycles, hops: warning.hopLimit }),
  ];
  // The server stops at a bound on what it names and on the work it does;
  // a list cut there must not read as every loop there is.
  if (warning.truncated) sentences.push(t("agents.eventCycleTruncated"));
  if (warning.cycles.some((agents) => agents.length === 1)) {
    sentences.push(t("agents.eventCycleSelf"));
  }
  return sentences.join(" ");
}
