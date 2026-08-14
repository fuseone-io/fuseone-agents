import type { AgentTrigger } from "@/lib/api/client";

/**
 * The three ways a run starts without a person.
 *
 * Manual is not one of them, and that is not an omission: an agent with no
 * trigger runs when somebody presses the button, which is how every agent
 * starts life while its author is still deciding whether to trust it. Offering
 * "manual" as a choice would make the honest default look like a configuration
 * somebody forgot to finish.
 */
export type TriggerKind = AgentTrigger["type"];

export const TRIGGER_KINDS: TriggerKind[] = ["cron", "webhook", "event"];

/** Which field a kind carries. */
export function fieldOf(kind: TriggerKind): "schedule" | "path" | "event" {
  switch (kind) {
    case "cron":
      return "schedule";
    case "webhook":
      return "path";
    default:
      return "event";
  }
}

/**
 * Whether a trigger would publish and then never fire.
 *
 * A schedule with no expression, a webhook with no path or an event with no
 * name are each a half-filled row that saves cleanly and does nothing — the
 * worst shape a trigger can have, because the screen says the agent is
 * triggered and nothing ever starts it.
 */
export function incomplete(trigger: AgentTrigger): boolean {
  return !String(trigger[fieldOf(trigger.type)] ?? "").trim();
}

/** A new row of the kind chosen, with nothing filled in. */
export function emptyTrigger(kind: TriggerKind): AgentTrigger {
  return { type: kind };
}
