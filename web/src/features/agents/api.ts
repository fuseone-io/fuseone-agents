import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { useScopeFilter } from "@/features/scope/use-scope-filter";
import type { components } from "@/lib/api/schema.gen";

export type Agent = components["schemas"]["Agent"];
export type AgentTrigger = components["schemas"]["AgentTrigger"];
export type AgentActivity = components["schemas"]["AgentActivity"];

export const agentKeys = {
  all: ["agents"] as const,
  list: (scope: string, allVersions: boolean) =>
    [...agentKeys.all, "list", scope, allVersions] as const,
};

/**
 * One row per agent by default. The publication history answers a different
 * question, and asking it by default would bury the current state under it.
 */
export function useAgents(allVersions = false) {
  const scope = useScopeFilter();
  return useQuery({
    queryKey: agentKeys.list(scope.key, allVersions),
    queryFn: async () =>
      unwrap(
        await api.GET("/agents", {
          params: { query: { ...scope.params, allVersions } },
        }),
      ),
  });
}
