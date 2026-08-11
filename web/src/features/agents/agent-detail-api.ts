import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { agentKeys } from "@/features/agents/api";

/**
 * One published version, with its history.
 *
 * The version travels in the key because two versions of the same agent are
 * different documents: a run pinned to an older one is explained by that one,
 * and a cache that conflated them would show an auditor text the run never ran
 * under.
 */
export function useAgent(agentId: string, version?: string) {
  return useQuery({
    queryKey: [...agentKeys.all, "detail", agentId, version ?? "latest"] as const,
    queryFn: async () =>
      unwrap(
        await api.GET("/agents/{agentId}", {
          params: { path: { agentId }, query: { version } },
        }),
      ),
  });
}
