import type { components } from "@/lib/api/schema.gen";

type AgentStep = components["schemas"]["AgentStep"];

/**
 * Whether the drawing says something the instructions do not.
 *
 * The two halves are authored separately on purpose — the prose is what the
 * model receives and the stages are what the Gate is meant to obey — which
 * means they can disagree, and a screen that never says so lets somebody
 * publish an agent whose text describes one process and whose permissions
 * describe another.
 *
 * The check is deliberately narrow and it is about tools, not about wording.
 * A stage that reaches `erp.transfer` with instructions that never mention
 * transferring is a real divergence in the direction that matters: the agent
 * is being allowed something nobody wrote down. The other direction — prose
 * describing a step nobody drew — is not flagged, because prose is allowed to
 * say more than the permissions do, and that is the safe way round.
 */
export function undescribed(steps: AgentStep[], instructions: string): string[] {
  const text = instructions.toLowerCase();

  const reached = new Set<string>();
  for (const step of steps) {
    for (const tool of step.reaches ?? []) reached.add(tool);
  }

  return [...reached].filter((tool) => {
    // The bare name as well as the qualified one: an author writing "look the
    // customer up with lookup" has described it, and demanding they paste an
    // identifier would make this fire on every well-written agent.
    const short = tool.includes(".") ? tool.slice(tool.indexOf(".") + 1) : tool;
    return !text.includes(tool.toLowerCase()) && !text.includes(short.toLowerCase());
  });
}
