import { useQueries } from "@tanstack/react-query";
import { toolsQuery, type Tool } from "@/features/admin/api";
import { agentTarget } from "@/features/agents/agent-target";
import type { Agent } from "@/features/agents/api";

/**
 * Each listed agent's catalogue, read at the agent's own scope (NT-019).
 *
 * One request per distinct scope among the agents given, which the caller
 * bounds to what is on the page: unscoped, an area's reader is answered the
 * installation's tools alone, and a card would read its area's tools as
 * nobody having ruled on them. Answers are shared with the editor and the
 * detail page, which ask the same keys.
 */
export function useScopedCatalogues(agents: Agent[], enabled = true) {
  const targets = [
    ...new Set(agents.map((agent) => agentTarget(agent.scope) ?? "")),
  ];
  const answers = useQueries({
    queries: targets.map((target) => ({
      ...toolsQuery(target || undefined),
      enabled,
    })),
  });
  const byTarget = new Map<string, Tool[] | undefined>(
    targets.map((target, i) => [target, answers[i]?.data?.items]),
  );
  return (agent: Agent): Tool[] | undefined =>
    byTarget.get(agentTarget(agent.scope) ?? "");
}
