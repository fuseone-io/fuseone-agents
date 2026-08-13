import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { agentKeys } from "@/features/agents/api";
import type { components } from "@/lib/api/schema.gen";

export type Stage = components["schemas"]["Stage"];

/**
 * Moves an agent between stages.
 *
 * Every list and detail is invalidated, because the stage decides whether the
 * agent can act at all: a screen still showing the old one is a screen telling
 * somebody their agent is running when it is not.
 */
export function useSetStage(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (stage: Stage) =>
      unwrap(
        await api.PUT("/agents/{agentId}/stage", {
          params: { path: { agentId } },
          body: { stage },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: agentKeys.all }),
  });
}
