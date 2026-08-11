import { effectOf, verdictOf } from "@/features/runs/step-verb";
import type { Step } from "@/lib/api/client";

export interface ToolUse {
  name: string;
  /** The call executed and its effect was not a read. */
  wrote: boolean;
  /** The gate stopped or constrained it at least once. */
  escalated: boolean;
}

/**
 * Which tools the run touched, and how.
 *
 * Built from the trail rather than the specification: what an agent is allowed
 * to call and what it actually called are different questions, and only the
 * second one is answerable after an incident.
 */
export function toolsOf(steps: Step[]): ToolUse[] {
  const uses = new Map<string, ToolUse>();

  for (const step of steps) {
    const payload = (step.payload ?? {}) as Record<string, unknown>;
    const name = payload.tool;
    if (typeof name !== "string") continue;

    const use = uses.get(name) ?? { name, wrote: false, escalated: false };
    // Whether anything actually changed is the first question after an
    // incident, so an unclassified effect counts as a read rather than as a
    // write: claiming a system was touched when it was not sends people
    // looking for damage that is not there.
    const effect = effectOf(step);
    if (step.kind === "tool_called" && effect !== undefined && effect !== "read" && effect !== "unknown") {
      use.wrote = true;
    }
    const verdict = verdictOf(step);
    if (verdict && verdict !== "allow") use.escalated = true;
    if (step.kind === "approval_requested") use.escalated = true;

    uses.set(name, use);
  }
  return [...uses.values()];
}
