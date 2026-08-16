import type { AgentTrigger } from "@/lib/api/client";

/**
 * The four ways a run starts without a person.
 *
 * Manual is not one of them, and that is not an omission: an agent with no
 * trigger runs when somebody presses the button, which is how every agent
 * starts life while its author is still deciding whether to trust it. Offering
 * "manual" as a choice would make the honest default look like a configuration
 * somebody forgot to finish.
 */
export type TriggerKind = AgentTrigger["type"];

export const TRIGGER_KINDS: TriggerKind[] = [
  "cron",
  "webhook",
  "event",
  "channel",
];

/**
 * Which field a kind carries, and `undefined` for the one that carries none.
 *
 * A channel trigger names no conversation. The author declares that an ask in
 * a conversation of their agent's scope may start it; which conversations
 * belong to which scope is administrative, and a field here would be the
 * author choosing who may start their own agent.
 */
export function fieldOf(
  kind: TriggerKind,
): "schedule" | "path" | "event" | undefined {
  switch (kind) {
    case "cron":
      return "schedule";
    case "webhook":
      return "path";
    case "event":
      return "event";
    default:
      return undefined;
  }
}

/**
 * Whether a trigger would publish and then never fire.
 *
 * A schedule with no expression, a webhook with no path or an event with no
 * name are each a half-filled row that saves cleanly and does nothing — the
 * worst shape a trigger can have, because the screen says the agent is
 * triggered and nothing ever starts it.
 *
 * A channel trigger has no field, so it is never half-filled. What it still
 * needs is a conversation mapped to this agent's scope, and that is somebody
 * else's screen — worth saying there rather than marking the row here as
 * unfinished by the person who cannot finish it.
 */
export function incomplete(trigger: AgentTrigger): boolean {
  const field = fieldOf(trigger.type);
  if (field === undefined) return false;
  return !String(trigger[field] ?? "").trim();
}

/** A new row of the kind chosen, with nothing filled in. */
export function emptyTrigger(kind: TriggerKind): AgentTrigger {
  return { type: kind };
}
