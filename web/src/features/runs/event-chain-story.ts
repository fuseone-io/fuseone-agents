import type { Line } from "@/features/runs/step-story";

/**
 * Where a run stands in a chain of runs that events opened one after
 * another, and what a run at the end of one did not publish (#203).
 *
 * The chain is sealed on run_started; the withheld record on run_finished,
 * by the run that reached the limit. Read defensively: a payload this cannot
 * read says nothing, rather than a sentence with holes in it.
 */

type RawRecord = Record<string, unknown>;

function asRecord(value: unknown): RawRecord | undefined {
  return typeof value === "object" && value !== null
    ? (value as RawRecord)
    : undefined;
}

export function eventChainLine(payload: RawRecord): Line | undefined {
  const chain = asRecord(payload.chain);
  if (!chain) return undefined;
  const { root, hop } = chain;
  if (typeof root !== "string" || typeof hop !== "number") return undefined;
  return { key: "runs.storyEventChain", values: { hop, root } };
}

export function eventsWithheldLine(payload: RawRecord): Line | undefined {
  const withheld = asRecord(payload.events_withheld);
  if (!withheld) return undefined;
  // Opened by an event that sealed no chain — an older dispatcher during a
  // rolling update — so there is no root to name.
  if (withheld.reason === "event_chain_unknown") {
    return { key: "runs.eventsWithheldChainUnknown" };
  }
  const { root, limit } = withheld;
  if (typeof root !== "string" || typeof limit !== "number") return undefined;
  return { key: "runs.eventsWithheld", values: { root, limit } };
}
