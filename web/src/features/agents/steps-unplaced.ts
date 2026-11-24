import type { components } from "@/lib/api/schema.gen";
import { PLATFORM_TOOLS } from "@/features/agents/platform-tools";

type AgentStep = components["schemas"]["AgentStep"];

/**
 * Each granted tool no step reaches, in the order granted.
 *
 * A run of an agent with steps is offered only what its steps reach, so these
 * never run. The server lists the same for the saved draft (`unplaced`) and
 * refuses to publish it from the workbench; this says it from what is on
 * screen, while the person edits and before anything is saved.
 */
export function unplacedTools(tools: string[], steps: AgentStep[]): string[] {
  if (steps.length === 0) return [];
  const placed = new Set(steps.flatMap((step) => step.reaches ?? []));
  return tools.filter((tool) => !placed.has(tool) && !PLATFORM_TOOLS.has(tool));
}
