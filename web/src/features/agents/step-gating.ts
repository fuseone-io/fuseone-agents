import { ruleFor } from "@/features/agents/tool-rule";
import type { Policy, Tool } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

type AgentStep = components["schemas"]["AgentStep"];

/**
 * Whether the Gate will do anything but allow what this stage reaches.
 *
 * One question asked in two views, so it is answered in one place: a strip
 * card and a text row disagreeing about whether a stage stops would be two
 * readings of one policy.
 *
 * Derived from the ladder and the policies in force, never from a setting on
 * the step. What the Gate does is stated here; only membership is editable.
 */
export function gateStops(
  step: AgentStep,
  catalogue: Tool[],
  policies: Policy[],
): boolean {
  return (step.reaches ?? []).some((tool) => {
    const effect = catalogue.find((one) => one.toolId === tool)?.effect ?? "write";
    return ruleFor(tool, effect, policies).kind !== "allowed";
  });
}
