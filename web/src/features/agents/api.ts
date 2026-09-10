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

/**
 * Who this agent may name to be told about its approvals.
 *
 * The narrow list, not the directory: naming somebody needs their name and
 * nothing else, and an author holds no authority over identity — the
 * administrative listing answered 403 and drew an empty control with no error.
 *
 * Asked only when a private message was actually asked for. A screen that reads
 * the people who may decide in order to draw a checkbox nobody ticked is a
 * screen making a request on every visit for a control that is not there.
 */
export function useEligibleApprovers(
  company: string,
  area: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: [...agentKeys.all, "approvers", company, area] as const,
    queryFn: async () =>
      unwrap(
        await api.GET("/agents/approvers", {
          params: { query: { company, area: area || undefined } },
        }),
      ),
    enabled: enabled && company !== "",
  });
}
