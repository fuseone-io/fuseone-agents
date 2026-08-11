import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { agentKeys } from "@/features/agents/api";

/** The paths an agent declares, and whether each can actually fire. */
export function useWebhooks(agentId: string) {
  return useQuery({
    queryKey: [...agentKeys.all, "webhooks", agentId] as const,
    queryFn: async () =>
      unwrap(await api.GET("/agents/{agentId}/webhooks", { params: { path: { agentId } } })),
  });
}

/**
 * Generates the secret, once.
 *
 * The response is the only time anybody sees it — what the platform keeps is a
 * hash — so the caller has to put it in front of a person immediately rather
 * than into a cache somebody might read later.
 */
export function useRotateWebhookSecret(agentId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (path: string) =>
      unwrap(
        await api.POST("/agents/{agentId}/webhooks/{path}/secret", {
          params: { path: { agentId, path } },
        }),
      ),
    onSuccess: () => {
      // Only the armed state is worth refetching. The secret is not stored
      // anywhere it could be refetched from, which is the point.
      void queryClient.invalidateQueries({ queryKey: [...agentKeys.all, "webhooks", agentId] });
    },
  });
}
