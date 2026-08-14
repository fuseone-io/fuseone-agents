import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { agentKeys } from "@/features/agents/api";
import type { AgentDefinition } from "@/lib/api/client";

/**
 * Publishes the next version of an agent.
 *
 * Creating and editing are one call, because editing is authoring the next
 * version: runs are pinned to versions and the older ones stay the only
 * correct explanation of the runs that used them.
 */
export function usePublishAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      agentId,
      definition,
    }: {
      agentId: string;
      definition: AgentDefinition;
    }) =>
      unwrap(
        await api.PUT("/agents/{agentId}/versions", {
          params: { path: { agentId } },
          body: definition,
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: agentKeys.all }),
  });
}

/** Starts or stops an agent. Not part of the definition and not versioned. */
export function useSetAgentPaused(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (paused: boolean) =>
      unwrap(
        await api.PUT("/agents/{agentId}/paused", {
          params: { path: { agentId } },
          body: { paused },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: agentKeys.all }),
  });
}

/**
 * Takes an agent out of circulation, or brings it back.
 *
 * There is no delete: a run is pinned to a version, and that version is the
 * only correct explanation of what the run did. A retired agent leaves the
 * lists and keeps everything.
 */
export function useSetAgentRetired(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (retired: boolean) =>
      unwrap(
        await api.PUT("/agents/{agentId}/retired", {
          params: { path: { agentId } },
          body: { retired },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: agentKeys.all }),
  });
}
