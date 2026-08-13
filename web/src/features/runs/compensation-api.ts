import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { components } from "@/lib/api/schema.gen";
import { api, unwrap } from "@/lib/api/client";
import { runKeys } from "@/features/runs/api";

export type CompensationAct = components["schemas"]["CompensationAct"];

/**
 * What undoing this run would do, before anybody does it.
 *
 * Only fetched when somebody opens the dialog: the plan is a read of the whole
 * trail plus the tool catalogue, and the answer to "what would this undo" is
 * not something every run page needs on load.
 */
export function useCompensationPlan(runId: string, enabled: boolean) {
  return useQuery({
    queryKey: [...runKeys.detail(runId), "compensation"],
    enabled,
    queryFn: async () =>
      unwrap(
        await api.GET("/runs/{runId}/compensation", {
          params: { path: { runId } },
        }),
      ),
  });
}

export function useAbandonRun(runId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { reason: string; compensate: boolean }) =>
      unwrap(
        await api.POST("/runs/{runId}/compensation", {
          params: { path: { runId } },
          body: input,
        }),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: runKeys.all });
    },
  });
}
