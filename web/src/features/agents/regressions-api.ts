import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { agentKeys } from "@/features/agents/api";
import type { components } from "@/lib/api/schema.gen";

export type RegressionCase = components["schemas"]["RegressionCase"];
export type Expectation = components["schemas"]["Expectation"];

export const regressionKeys = {
  of: (agentId: string) => [...agentKeys.all, "regressions", agentId] as const,
};

/** The corrections this agent is held to, re-checked on every version. */
export function useRegressions(agentId: string) {
  return useQuery({
    enabled: agentId !== "",
    queryKey: regressionKeys.of(agentId),
    queryFn: async () =>
      unwrap(
        await api.GET("/agents/{agentId}/regressions", {
          params: { path: { agentId } },
        }),
      ),
  });
}

export function useRecordRegression(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      runId: string;
      expectations: Expectation[];
      note?: string;
    }) =>
      unwrap(
        await api.POST("/agents/{agentId}/regressions", {
          params: { path: { agentId } },
          body: input,
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({
        queryKey: regressionKeys.of(agentId),
      }),
  });
}

export function useDeleteRegression(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (caseId: string) =>
      unwrap(
        await api.DELETE("/agents/{agentId}/regressions/{caseId}", {
          params: { path: { agentId, caseId } },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({
        queryKey: regressionKeys.of(agentId),
      }),
  });
}
